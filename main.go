package main
import "os"

func main( ) {
	os.Exit( func( )int{
		if len( os.Args ) < 2 {
			return 2
		}
		switch os.Args[ 1 ] {
			case "client" :
				return client( os.Args[ 2 : ] )
			case "server" :
				return server( os.Args[ 2 : ] )
			default :
				return 2
		}
	}( ) )
}

