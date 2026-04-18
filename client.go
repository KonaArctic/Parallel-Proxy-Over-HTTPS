package main
import crand "crypto/rand"
import "crypto/tls"
import "encoding/binary"
import "fmt"
import "io"
import "net"
import "net/http"
import "net/netip"
import "net/url"
import mrand "math/rand"
import "os"
import "slices"
import "strings"
import "time"

var nocert bool = func( )bool{
	switch strings.ToLower( os.Getenv( "KONA_INSECURE_SKIP_VERIFY" ) ) {
		case "1" :
			return true
		case "true" :
			return true
		case "yes" :
			return true
		default :
			return false
	}
}( )

func client( argues [ ]string )int {
	var err error
	if len( argues ) < 2 {
		return 2
	}
	var listen net.Listener
	_ , _ , err = net.SplitHostPort( argues[ 0 ] )
	if err != nil {
		return 2
	}
	listen , err = net.Listen( "tcp" , argues[ 0 ] )
	if err != nil {
		return 3
	}
	defer listen.Close( )
	var pxyurl * url.URL
	pxyurl , err = url.Parse( argues[ 1 ] )
	if err != nil {
		return 2
	}
	var pooler chan io.ReadWriteCloser = make( chan io.ReadWriteCloser , 0 )
	for _ , _ = range make( [ ]any , 256 , 256 ) {
		go func( ){
			var err error
			for {
				var respon * http.Response
				respon , err = ( & http.Client{
					Transport : & http.Transport{
						DialContext : ( & net.Dialer{
							Timeout : time.Second * 30 ,
							KeepAlive : time.Second * 30 ,
						} ).DialContext ,
						MaxIdleConns : 100 ,
						IdleConnTimeout : time.Second * 90 ,
						TLSHandshakeTimeout : time.Second * 10 ,
						ExpectContinueTimeout : time.Second * 1 ,
						TLSClientConfig : & tls.Config{
							InsecureSkipVerify : nocert ,
						} ,
						TLSNextProto : map[ string ]func( string , * tls.Conn )http.RoundTripper{ } ,
					} ,
				} ).Do( & http.Request{
					Method : http.MethodGet ,
					URL : pxyurl ,
					Header : map[ string ][ ]string{
						"Connection" : [ ]string{
							"upgrade" ,
						} ,
						"Upgrade" : [ ]string{
							"KAPPOH/0.1" ,
						} ,
					} ,
				} )
				if err != nil {
					_ , _ = fmt.Fprintf( os.Stderr , "Err: %v\r\n" , err )
					time.Sleep( time.Second )
					continue
				}
				if respon.StatusCode != http.StatusSwitchingProtocols {
					_ , _ = fmt.Fprintf( os.Stderr , "Err: server returned %v %v\r\n" , respon.StatusCode , respon.Status )
					time.Sleep( time.Minute )
					continue
				}
				var stream io.ReadWriteCloser
				stream = respon.Body.( io.ReadWriteCloser )
				inners : for {
					select {
						case pooler <- stream :
							break inners
						case <- time.After( time.Second * 50 ) :
							_ , err = stream.Write( [ ]byte{
								byte( mrand.Int( ) % 255 ) + 1 ,
							} )
							if err != nil {
								break inners
							}
					}
				}
			}
		}( )
	}
	var authed map[ netip.Addr ]time.Time = map[ netip.Addr ]time.Time{ }
	var locker chan any = make( chan any , 1 )
	err = ( & http.Server{
		Handler : http.HandlerFunc( func( respon http.ResponseWriter , reques * http.Request ){
			var err error
			var ok bool
			if reques.URL.Host == "" {
				respon.Header( )[ "Location" ] = [ ]string{
					( & url.URL{
						Scheme : "http" ,
						Host : reques.Host ,
					} ).String( ) ,
				}
				respon.WriteHeader( http.StatusUseProxy )
				_ , _ = respon.Write( [ ]byte( "This is a HTTP proxy\r\n" ) )
				return
			}
			// Im too lazy to implement a plain HTTP proxy
			if reques.Method != http.MethodConnect {
				respon.Header( )[ "Location" ] = [ ]string{
					( & url.URL{
						Scheme : "https" ,
						Host : reques.URL.Host ,
						RawPath : reques.URL.RawPath ,
						RawQuery : reques.URL.RawQuery ,
					} ).String( ) ,
				}
				respon.WriteHeader( http.StatusTemporaryRedirect )
				_ , _ = respon.Write( [ ]byte( "Upgrading to HTTPS ...\r\n" ) )
				return
			}
			_ , _ = fmt.Fprintf( os.Stderr , "Info: %v %v\r\n" , reques.RemoteAddr , reques.URL.Host )
			var ipaddr netip.AddrPort
			ipaddr , err = netip.ParseAddrPort( reques.RemoteAddr )
			if err != nil {
				respon.WriteHeader( http.StatusInternalServerError )
				_ , _ = respon.Write( [ ]byte( err.Error( ) + "\r\n" ) )
				return
			}
			// Allow local area network
			var inface [ ]net.Addr 
			inface , err = net.InterfaceAddrs( )
			if err != nil {
				respon.WriteHeader( http.StatusInternalServerError )
				_ , _ = respon.Write( [ ]byte( err.Error( ) + "\r\n" ) )
				return
			}
			var remote bool = true
			for ; len( inface ) > 0 ; inface = inface[ 1 : ] {
				var prefix netip.Prefix
				prefix , err = netip.ParsePrefix( inface[ 0 ].String( ) )
				if err != nil {
					respon.WriteHeader( http.StatusInternalServerError )
					_ , _ = respon.Write( [ ]byte( err.Error( ) + "\r\n" ) )
					return
				}
				if prefix.Contains( ipaddr.Addr( ) ) {
					remote = false
				}
			}
			if remote {
				// Check we previously authorized this clients address
				// Insecure, but allows user-agents that lack proxy authorization
				var latest time.Time
				locker <- true
				latest , ok = authed[ ipaddr.Addr( ) ]
				if  ! ok ||
				    time.Now( ).Sub( latest ) > time.Hour {
					var header [ ]string
					header , ok = reques.Header[ "Proxy-Authorization" ]
					if ! ok {
					    	<- locker
						respon.Header( )[ "Proxy-Authenticate" ] = [ ]string{
							"Basic" ,
						}
						respon.WriteHeader( http.StatusProxyAuthRequired )
						_ , _ = respon.Write( [ ]byte( "Proxy password required\r\n" ) )
						return
					}
					var passwd string
					_ , passwd , ok = ( & http.Request{
						Header : map[ string ][ ]string{
							"Authorization" : header ,
						} ,
					} ).BasicAuth( )
					if  ! ok ||
					    ! slices.Contains( argues[ 2 : ] , passwd ) {
					    	<- locker
						respon.Header( )[ "Proxy-Authenticate" ] = [ ]string{
							"Basic" ,
						}
						respon.WriteHeader( http.StatusProxyAuthRequired )
						_ , _ = respon.Write( [ ]byte( "Wrong password\r\n" ) )
						return
					}
					
				}
				authed[ ipaddr.Addr( ) ] = time.Now( )
				<- locker
			}
			var stream io.ReadWriteCloser = nil
			select {
				case stream = <- pooler :
				default :
					_ , _ = fmt.Fprintf( os.Stderr , "Err: Pool is empty!\r\n" )
					respon.WriteHeader( http.StatusBadGateway )
					_ , _ = respon.Write( [ ]byte( "Cannot connect to server\r\n" ) )
					return
			}
			defer stream.Close( )
			var idcode [ ]byte = make( [ ]byte , 16 , 16 )
			_ , err = io.ReadFull( crand.Reader , idcode )
			if err != nil {
				respon.WriteHeader( http.StatusInternalServerError )
				_ , _ = respon.Write( [ ]byte( err.Error( ) + "\r\n" ) )
				return
			}
			_ , err = stream.Write( append( append( append( [ ]byte{
				0x00 ,
			} , idcode ... ) , uint8( len( [ ]byte( reques.URL.Host ) ) ) ) , [ ]byte( reques.URL.Host ) ... ) )
			if err != nil {
				respon.WriteHeader( http.StatusBadGateway )
				_ , _ = respon.Write( [ ]byte( err.Error( ) + "\r\n" ) )
				return
			}
			var hijack http.Hijacker
			hijack , ok = respon.( http.Hijacker )
			if ! ok {
				respon.WriteHeader( http.StatusInternalServerError )
				_ , _ = respon.Write( [ ]byte( "http.ResponseWriter does not implement http.Hijacker\r\n" ) )
				return
			}
			var writer io.WriteCloser
			var reader io.Reader
			writer , reader , err = hijack.Hijack( )
			if err != nil {
				respon.WriteHeader( http.StatusInternalServerError )
				_ , _ = respon.Write( [ ]byte( err.Error( ) + "\r\n" ) )
				return
			}
			defer writer.Close( )
			err = ( & http.Response{
				ProtoMajor : 1 ,
				ProtoMinor : 1 ,
				StatusCode : http.StatusOK ,
				ContentLength : -1 ,
				Header : map[ string ][ ]string{
					"Connection" : [ ]string{ } ,
					"Date" : [ ]string{
						time.Now( ).Format( http.TimeFormat ) ,
					} ,
				} ,
			} ).Write( writer )
			if err != nil {
				return
			}
			var finish chan any = make( chan any , 0 )
			go func( ){
				_ , _ = io.Copy( stream , reader )
				finish <- true
			}( )
			go func( ){
				var err error
				var ok bool
				var groups [ ]io.ReadCloser = [ ]io.ReadCloser{
					stream ,
				}
				var queued map[ uint8 ]io.ReadCloser = map[ uint8 ]io.ReadCloser{ }
				outers : for {
					var length uint16
					err = binary.Read( groups[ 0 ] , binary.BigEndian , & length )
					if err != nil {
						break
					}
					if length == 0 {
						var status uint16
						err = binary.Read( groups[ 0 ] , binary.BigEndian , & status )
						if err != nil {
							break
						}
						if status < 256 {
							var reader io.ReadCloser
							reader , ok = queued[ uint8( status ) ]
							if ok {
								groups = append( groups , reader )
								_ , _ = fmt.Fprintf( os.Stderr , "Info: %X now has %v connections\r\n" , idcode[ : 4 ] , len( groups ) )
								delete( queued , uint8( status ) )
							}
						}
						if status > 756 {
							_ , _ = fmt.Fprintf( os.Stderr , "Warn: Write on %X from %v took %vms\r\n" , idcode[ : 4 ] , reques.URL.Host , status - 256 )
							// Im assuming applications read quickly and dont become bottlenecks
							if  len( queued ) == 0 && 
							    ! remote {
								// Quickly satisfy demand
							    	for len( queued ) < map[ int ]int{
							    		1 : 15 ,
							    		16 : 48 ,
							    		64 : 64 ,
							    		128 : 0 ,
							    	}[ len( groups ) ] {
							    		var stream io.ReadWriteCloser = nil
							    		select {
										case stream = <- pooler :
										default :
											_ , _ = fmt.Fprintf( os.Stderr , "Err: Pool is empty!\r\n" )
											break outers
									}
									defer stream.Close( )
									_ , err = stream.Write( append( append( [ ]byte{
										0x00 ,
									} , idcode ... ) , uint8( len( queued ) ) ) )
									if err != nil {
										break outers
									}
									queued[ uint8( len( queued ) ) ] = stream
								}
							}								
						}
					} else {
						_ , err = io.Copy( writer , & io.LimitedReader{
							N : int64( length ) ,
							R : groups[ 0 ] ,
						} )
						if err != nil {
							break
						}
						groups = append( groups , groups[ 0 ] )[ 1 : ] 
					}
				}
				finish <- true
			}( )
			<- finish
		} ) ,
	} ).Serve( listen )
	if err != nil {
		return 1
	}
	return 0
}

