Parallel Proxy Over HTTPS
=========================
Some internet service providers and firewalls throttles connections, but for some reason only each TCP connection independently. Therefore we can workaround this by using a tunnelling proxy that create hundreds of real TCP connections for each tunnelled connection ... 

Usage
-----
Parallel Proxy Over HTTPS is known to work with [OVH](https://ovhcloud.com) and [Akile](https://akile.io), and is expected to work with any other [VPS](https://en.wikipedia.org/wiki/Virtual_Private_Server) provider, and also with reverse proxies like [Cloudflare](https://cloudflare.com). If `HOSTNAME` is your server's hostname, and `PASSWORD` is a randomly generated password.

-	On your  run `go run $(pwd) server :443 PASSWORD`
-	On your client run `KONA_INSECURE_SKIP_VERIFY=true go run $(pwd) client :53554 https://:PASSWORD@HOSTNAME`
-	Then point your system's HTTP proxy to `http://localhost:53554`

License 
-------
Copyright (C) 2026 Kona Arctic

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
