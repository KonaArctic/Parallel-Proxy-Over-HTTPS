package main
import "crypto/tls"
import "encoding/binary"
import "golang.org/x/net/proxy"
import "io"
import "net"
import "net/http"
import "slices"
import "strings"
import "time"

var dialer proxy.Dialer = proxy.FromEnvironment( )

func server( argues [ ]string )int {
	var err error
	if len( argues ) < 2 {
		return 2
	}
	var listen net.Listener
	listen , err = net.Listen( "tcp" , argues[ 0 ] )
	if err != nil {
		return 3
	}
	defer listen.Close( )
	// Im too lazy to implement proper certificates ...
	var crtkey tls.Certificate
	crtkey , err = tls.X509KeyPair( [ ]byte( `-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----` ) , [ ]byte( `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIIrYSSNQFaA2Hwf1duRSxKtLYX5CB04fSeQ6tF1aY/PuoAoGCCqGSM49
AwEHoUQDQgAEPR3tU2Fta9ktY+6P9G0cWO+0kETA6SFs38GecTyudlHz6xvCdz8q
EKTcWGekdmdDPsHloRNtsiCa697B2O9IFA==
-----END EC PRIVATE KEY-----` ) )
	if err != nil {
		return 1
	}
	var id2que map[ string ]( chan io.ReadWriteCloser ) = map[ string ]( chan io.ReadWriteCloser ){ }
	var locker chan any = make( chan any , 1 )
	err = ( & http.Server{
		Handler : http.HandlerFunc( func( respon http.ResponseWriter , reques * http.Request ){
			var err error
			var ok bool
			if  reques.Method != http.MethodHead &&
			    reques.Method != http.MethodGet {
				respon.WriteHeader( http.StatusNotImplemented )
				_ , _ = respon.Write( [ ]byte( http.StatusText( http.StatusNotImplemented ) + "\r\n" ) )
				return
			}
			var passwd string
			_ , passwd , ok = reques.BasicAuth( )
			if  ! ok ||
			    ! slices.Contains( argues[ 1 : ] , passwd ) {
				respon.Header( )[ "WWW-Authenticate" ] = [ ]string{
					"Basic" ,
				}
				respon.WriteHeader( http.StatusUnauthorized )
				_ , _ = respon.Write( [ ]byte( "Please use your password\r\n" ) )
				return
			}
			var values [ ]string
			values , ok = reques.Header[ "Upgrade" ]
			if ok {
				for ; len( values ) > 0 ; values = values[ 1 : ] {
					var splits [ ]string
					splits = strings.Split( values[ 0 ] , "," )
					for ; len( splits ) > 0 ; splits = splits[ 1 : ] {
						if strings.ToUpper( strings.TrimSpace( splits[ 0 ] ) ) == "KAPPOH/0.1" {
							goto upgrade
						}
					}
				}
			}
			respon.Header( )[ "Upgrade" ] = [ ]string{
				"KAPPOH/0.1" ,
			}
			respon.WriteHeader( http.StatusUpgradeRequired )
			_ , _ = respon.Write( [ ]byte( "This is a KAPPOH server\r\n" ) )
			return
			upgrade:
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
				ProtoMajor : reques.ProtoMajor ,
				ProtoMinor : reques.ProtoMinor ,
				StatusCode : http.StatusSwitchingProtocols ,
				Header : map[ string ][ ]string{
					"Connection" : [ ]string{
						"Upgrade" ,
					} ,
					"Date" : [ ]string{
						time.Now( ).Format( http.TimeFormat ) ,
					} ,
					"Upgrade" : [ ]string{
						"KAPPOH/0.1" ,
					} ,
				} ,
			} ).Write( writer )
			if err != nil {
				return
			}
			if reques.Method == http.MethodHead {
				return
			}
			for {
				var buffer [ ]byte = make( [ ]byte , 1 , 1 )
				_ , err = reader.Read( buffer )
				if err != nil {
					return
				}
				if buffer[ 0 ] == 0x00 {
					break
				}
			}
			var idcode [ ]byte = make( [ ]byte , 16 , 16 )
			_ , err = io.ReadFull( reader , idcode )
			if err != nil {
				return 
			}
			var queues chan io.ReadWriteCloser
			locker <- true
			queues , ok = id2que[ string( idcode ) ]
			if ok {
				<- locker
				var finish chan any = make( chan any , 1 )
				// FIXME May leak goroutines
				queues <- struct{
					io.Reader
					io.Writer
					io.Closer
				}{
					Reader : reader ,
					Writer : writer ,
					Closer : closer( func( )error{
						finish <- true
						return writer.Close( )
					} ) ,
				}
				<- finish 
			} else {
				queues = make( chan io.ReadWriteCloser , 0 )
				id2que[ string( idcode ) ] = queues
				<- locker
				defer func( ){
					locker <- true
					delete( id2que, string( idcode ) )
					<- locker 
				}( )
				var length [ ]byte = make( [ ]byte , 1 , 1 )
				_ , err = reader.Read( length )
				if err != nil {
					return
				}
				var buffer [ ]byte
				buffer = make( [ ]byte , length[ 0 ] , length[ 0 ] )
				_ , err = io.ReadFull( reader , buffer )
				if err != nil {
					return
				}
				var stream io.ReadWriteCloser
				stream , err = dialer.Dial( "tcp" , string( buffer ) )
				if err != nil {
					return
				}
				defer stream.Close( )
				var finish chan any = make( chan any , 2 )
				go func( ){
					_ , _ = io.Copy( stream , reader )
					finish <- true
				}( )
				go func( ){
					var err error
					var groups [ ]io.WriteCloser = [ ]io.WriteCloser{
						writer ,
					}
					outers : for {
						var buffer [ ]byte = make( [ ]byte , 65535 , 65535 )
						var length int
						length , err = stream.Read( buffer )
						if err != nil {
							break
						}
						var header [ ]byte
						header , err = binary.Append( header , binary.BigEndian , uint16( length ) )
						if err != nil {
							break
						}
						var starts time.Time
						starts = time.Now( )
						_ , err = groups[ 0 ].Write( append( header , buffer[ : length ] ... ) )
						var delays time.Duration
						delays = time.Now( ).Sub( starts )
						if err != nil {
							break
						}
						groups = append( groups , groups[ 0 ] )[ 1 : ]
						if delays > time.Millisecond * 100 {
							err = binary.Write( groups[ 0 ] , binary.BigEndian , struct{
								Length uint16
								Status uint16
							}{ 
								Status : uint16( delays / time.Millisecond ) + 256 ,
							} )
							if err != nil {
								break 
							}
						}
						inners : for {
							var stream io.ReadWriteCloser
							select {
								case stream = <- queues :
									defer stream.Close( )
									var replys [ ]byte = make( [ ]byte , 1 , 1 )
									_ , err = stream.Read( replys )
									if err != nil {
										break outers
									}
									groups = append( groups , stream )
									err = binary.Write( groups[ 0 ] , binary.BigEndian , struct{
										Length uint16
										Status uint16
									}{ 
										Status : uint16( replys[ 0 ] ) ,
									} )
									if err != nil {
										break outers
									}
								default :
									break inners
							}
						}
					}
					finish <- true
				}( )
				<- finish
			}
		} ) ,
	} ).Serve( tls.NewListener( listen , & tls.Config{
		Certificates : [ ]tls.Certificate{
			crtkey ,
		} ,
	} ) )
	if err != nil {
		return 1
	}
	return 0
}

type closer func( )error

func ( self closer )Close( )error {
	return self( )
}

/*
// Detect and unwrap TLS
type detect struct{
	net.Listener
}

func ( self * detect )Accept( )( net.Conn , error ) {
	var err error
	var stream net.Conn
	stream , err = self.Listener.Accept( )
	if err != nil {
		return nil , err
	}
	err = stream.SetReadDeadline( time.Now( ).Add( time.Minute ) )
	if err != nil {
		_ = stream.Close( )
		return self.Accept( )
	}
	var buffer [ ]byte = make( [ ]byte , 1 , 1 )
	_ , err = stream.Read( buffer )
	if err != nil {
		_ = stream.Close( )
		return self.Accept( )
	}
	// Hmm ...
	type noread interface{
		io.WriteCloser
		LocalAddr( )net.Addr
		RemoteAddr( )net.Addr
		SetDeadline( time.Time)error
		SetReadDeadline( time.Time )error
		SetWriteDeadline( time.Time )error
	}
	stream = struct{
		noread
		io.Reader
	}{
		noread : stream ,
		Reader : io.MultiReader( bytes.NewReader( buffer ) , stream ) ,
	}
	// TLS starts with 0x16
	if  buffer[ 0 ] < 0x20 ||
	    buffer[ 0 ] > 0xFE {
		// Im too lazy to implement proper certificates ...
		var crtkey tls.Certificate
		crtkey , err = tls.X509KeyPair( [ ]byte( `-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----` ) , [ ]byte( `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIIrYSSNQFaA2Hwf1duRSxKtLYX5CB04fSeQ6tF1aY/PuoAoGCCqGSM49
AwEHoUQDQgAEPR3tU2Fta9ktY+6P9G0cWO+0kETA6SFs38GecTyudlHz6xvCdz8q
EKTcWGekdmdDPsHloRNtsiCa697B2O9IFA==
-----END EC PRIVATE KEY-----` ) )
		if err != nil {
			_ = stream.Close( )
			return nil , err
		}
	    	return tls.Server( stream , & tls.Config{
			Certificates : [ ]tls.Certificate{
				crtkey ,
			} ,
	    	} ) , nil
	} else {
		return stream , nil
	}
} 

/*	var tcpcon * net.TCPConn
	tcpcon , ok = stream.( * net.TCPConn )
	if ok {
		var length int
		for length = 4096 ; length < 33554432 ; length *= 2 {
			err = tcpcon.SetWriteBuffer( length )
			if err != nil {
				break
			}
		}
	}	*/

