package netutil

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/gobwas/ws"
	"github.com/net-byte/go-gateway"
	"github.com/net-byte/vtun/common/config"
	"github.com/net-byte/vtun/common/counter"
)

// ConnectServer connects to the server with the given address.
func ConnectServer(config config.Config) net.Conn {
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, net.JoinHostPort(config.DNSIP, "53"))
		},
	}
	scheme := "ws"
	if config.Protocol == "wss" {
		scheme = "wss"
	}
	u := url.URL{Scheme: scheme, Host: config.ServerAddr, Path: config.WebSocketPath}
	header := make(http.Header)
	header.Set("user-agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/88.0.4324.182 Safari/537.36")
	header.Set("key", config.Key)
	tlsconfig := &tls.Config{
		InsecureSkipVerify: config.TLSInsecureSkipVerify,
	}
	dialer := ws.Dialer{
		Header:    ws.HandshakeHeaderHTTP(header),
		Timeout:   time.Duration(config.Timeout) * time.Second,
		TLSConfig: tlsconfig,
	}
	c, _, _, err := dialer.Dial(context.Background(), u.String())
	if err != nil {
		log.Printf("[client] failed to dial websocket %s %v", u.String(), err)
		return nil
	}
	return c
}

// GetInterfaceName returns the name of interface
func GetInterface() (name string) {
	ifaces := getAllInterfaces()
	if len(ifaces) == 0 {
		return ""
	}
	netAddrs, _ := ifaces[0].Addrs()
	for _, addr := range netAddrs {
		ip, ok := addr.(*net.IPNet)
		if ok && ip.IP.To4() != nil && !ip.IP.IsLoopback() {
			name = ifaces[0].Name
			break
		}
	}
	return name
}

// getAllInterfaces returns all interfaces
func getAllInterfaces() []net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Println(err)
		return nil
	}

	var outInterfaces []net.Interface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback == 0 && iface.Flags&net.FlagUp == 1 && isPhysicalInterface(iface.Name) {
			netAddrs, _ := iface.Addrs()
			if len(netAddrs) > 0 {
				outInterfaces = append(outInterfaces, iface)
			}
		}
	}
	return outInterfaces
}

// isPhysicalInterface returns true if the interface is physical
func isPhysicalInterface(addr string) bool {
	prefixArray := []string{"ens", "enp", "enx", "eno", "eth", "en0", "wlan", "wlp", "wlo", "wlx", "wifi0", "lan0"}
	for _, pref := range prefixArray {
		if strings.HasPrefix(strings.ToLower(addr), pref) {
			return true
		}
	}
	return false
}

// Lookup IP address of the given hostname
func LookupIP(domain string) net.IP {
	ips, err := net.LookupIP(domain)
	if err != nil || len(ips) == 0 {
		log.Println(err)
		return nil
	}
	return ips[0]
}

// IsIPv4 returns true if the packet is IPv4s
func IsIPv4(packet []byte) bool {
	flag := packet[0] >> 4
	return flag == 4
}

// IsIPv6 returns true if the packet is IPv6s
func IsIPv6(packet []byte) bool {
	flag := packet[0] >> 4
	return flag == 6
}

// GetIPv4Src returns the IPv4 source address of the packet
func GetIPv4Src(packet []byte) net.IP {
	return net.IPv4(packet[12], packet[13], packet[14], packet[15])
}

// GEtIPv4Dst returns the IPv4 destination address of the packet
func GetIPv4Dst(packet []byte) net.IP {
	return net.IPv4(packet[16], packet[17], packet[18], packet[19])
}

// GetIPv6Src returns the IPv6 source address of the packet
func GetIPv6Src(packet []byte) net.IP {
	return net.IP(packet[8:24])
}

// GetIPv6Dst returns the IPv6 destination address of the packet
func GetIPv6Dst(packet []byte) net.IP {
	return net.IP(packet[24:40])
}

// GetSrcKey returns the source key of the packet
func GetSrcKey(packet []byte) string {
	key := ""
	if IsIPv4(packet) && len(packet) >= 20 {
		key = GetIPv4Src(packet).To4().String()
	} else if IsIPv6(packet) && len(packet) >= 40 {
		key = GetIPv6Src(packet).To16().String()
	}
	return key
}

// GetdstKey returns the destination key of the packets
func GetDstKey(packet []byte) string {
	key := ""
	if IsIPv4(packet) && len(packet) >= 20 {
		key = GetIPv4Dst(packet).To4().String()
	} else if IsIPv6(packet) && len(packet) >= 40 {
		key = GetIPv6Dst(packet).To16().String()
	}
	return key
}

// ExecuteCommand executes the given command
func ExecCmd(c string, args ...string) string {
	log.Printf("exec %v %v", c, args)
	cmd := exec.Command(c, args...)
	out, err := cmd.Output()
	if err != nil {
		log.Println("failed to exec cmd:", err)
	}
	if len(out) == 0 {
		return ""
	}
	s := string(out)
	return strings.ReplaceAll(s, "\n", "")
}

// DiscoverGateway returns the local gateway IP address
func DiscoverGateway(ipv4 bool) string {
	var ip net.IP
	var err error
	if ipv4 {
		ip, err = gateway.DiscoverGatewayIPv4()
	} else {
		ip, err = gateway.DiscoverGatewayIPv6()
	}
	if err != nil {
		log.Println(err)
		return ""
	}
	return ip.String()
}

// LookupServerAddrIP returns the IP of server address
func LookupServerAddrIP(serverAddr string) net.IP {
	host, _, err := net.SplitHostPort(serverAddr)
	if err != nil {
		log.Panic("error server address")
		return nil
	}
	ip := LookupIP(host)
	return ip
}

// GetDefaultHttpResponse returns the default http response
func GetDefaultHttpResponse() []byte {
	return []byte(`HTTP/1.1 200 OK
	Server: nginx/1.18.0 (Ubuntu)
	Content-Type: text/html
	Content-Length: 612
	Connection: keep-alive
	Accept-Ranges: bytes
	
	<!DOCTYPE html>
	<html>
	<head>
	<title>Welcome to nginx!</title>
	<style>
		body {
			width: 35em;
			margin: 0 auto;
			font-family: Tahoma, Verdana, Arial, sans-serif;
		}
	</style>
	</head>
	<body>
	<h1>Welcome to nginx!</h1>
	<p>If you see this page, the nginx web server is successfully installed and
	working. Further configuration is required.</p>
	
	<p>For online documentation and support please refer to
	<a href="http://nginx.org/">nginx.org</a>.<br/>
	Commercial support is available at
	<a href="http://nginx.com/">nginx.com</a>.</p>
	
	<p><em>Thank you for using nginx.</em></p>
	</body>
	</html>`)
}

// PrintErr returns the error log
func PrintErr(err error, enableVerbose bool) {
	if !enableVerbose {
		return
	}
	log.Printf("error:%v", err)
}

// PrintStats returns the stats info
func PrintStats(enableVerbose bool, serverMode bool) {
	if !enableVerbose {
		return
	}
	go func() {
		for {
			time.Sleep(30 * time.Second)
			log.Printf("stats:%v", counter.PrintBytes(serverMode))
		}
	}()
}
