package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	flag "github.com/spf13/pflag"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	ProtocolICMP     = 1
	ProtocolIPv6ICMP = 58
)

var (
	flagTimeout = flag.DurationP("timeout", "t", 5*time.Second, "time to wait for a reply")
	flagListen4 = flag.String("listen4", "0.0.0.0", "listen address for IPv4 sockets")
	flagListen6 = flag.String("listen6", "::", "listen address for IPv6 sockets")
	flagData    = flag.BytesHexP("data", "d", []byte{}, "data to send in the request, as hex bytes")
)

func main() {
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s [ADDR...]\n", os.Args[0])
		os.Exit(1)
	}

	for _, addr := range args {
		if err := ping(addr); err != nil {
			log.Printf("[%s] error: %v", addr, err)
		}
	}
}

func ping(addr string) error {
	addrs, err := net.LookupHost(addr)
	if err != nil {
		return fmt.Errorf("error resolving %q: %v", addr, err)
	}

	var wg sync.WaitGroup
	for _, raddr := range addrs {
		ip := net.ParseIP(raddr)
		if ip == nil {
			log.Printf("[%s] error parsing address %q: %v", addr, raddr, err)
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			if ip4 := ip.To4(); ip4 != nil {
				ipa := &net.IPAddr{IP: ip4}
				if err := ping4(addr, ipa); err != nil {
					log.Printf("[%s] error: %v", addr, err)
				}

			} else if ip6 := ip.To16(); ip6 != nil {
				ipa := &net.IPAddr{IP: ip6}
				if err := ping6(addr, ipa); err != nil {
					log.Printf("[%s] error: %v", addr, err)
				}

			} else {
				log.Printf("[%s] unexpected IP type", addr)
			}
		}()
	}

	wg.Wait()
	return nil
}

func ping4(addr string, resolved *net.IPAddr) error {
	c, err := icmp.ListenPacket("ip4:icmp", *flagListen4)
	if err != nil {
		return err
	}
	defer c.Close()

	m := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   os.Getpid() & 0xffff,
			Seq:  1,
			Data: *flagData,
		},
	}
	b, err := m.Marshal(nil)
	if err != nil {
		return err
	}

	start := time.Now()
	n, err := c.WriteTo(b, resolved)
	if err != nil {
		return err
	} else if n != len(b) {
		return fmt.Errorf("got %v; want %v", n, len(b))
	}

	reply := make([]byte, 1500)
	err = c.SetReadDeadline(time.Now().Add(*flagTimeout))
	if err != nil {
		return err
	}
	n, peer, err := c.ReadFrom(reply)
	duration := time.Since(start)
	if err != nil {
		if opErr, ok := err.(*net.OpError); ok {
			if opErr.Timeout() {
				log.Printf("[%s] %s: request timeout in %s", addr, resolved.IP, duration)
				return nil
			}
		}
		return err
	}

	rm, err := icmp.ParseMessage(ProtocolICMP, reply[:n])
	if err != nil {
		return err
	}
	switch rm.Type {
	case ipv4.ICMPTypeEchoReply:
		log.Printf("[%s] %s: got reply in %s", addr, resolved.IP, duration)

	case ipv4.ICMPTypeDestinationUnreachable:
		log.Printf("[%s] %s: destination unreachable in %s", addr, resolved.IP, duration)

	default:
		return fmt.Errorf("got %+v from %v; want echo reply", rm, peer)
	}

	return nil
}

func ping6(addr string, resolved *net.IPAddr) error {
	c, err := icmp.ListenPacket("ip6:icmp", *flagListen6)
	if err != nil {
		return err
	}
	defer c.Close()

	m := icmp.Message{
		Type: ipv6.ICMPTypeEchoRequest,
		Code: 0,
		Body: &icmp.Echo{
			ID:   os.Getpid() & 0xffff,
			Seq:  1,
			Data: *flagData,
		},
	}
	b, err := m.Marshal(nil)
	if err != nil {
		return err
	}

	start := time.Now()
	n, err := c.WriteTo(b, resolved)
	if err != nil {
		return err
	} else if n != len(b) {
		return fmt.Errorf("got %v; want %v", n, len(b))
	}

	reply := make([]byte, 1500)
	err = c.SetReadDeadline(time.Now().Add(*flagTimeout))
	if err != nil {
		return err
	}
	n, peer, err := c.ReadFrom(reply)
	duration := time.Since(start)
	if err != nil {
		if opErr, ok := err.(*net.OpError); ok {
			if opErr.Timeout() {
				log.Printf("[%s] %s: request timeout in %s", addr, resolved.IP, duration)
				return nil
			}
		}
		return err
	}

	rm, err := icmp.ParseMessage(ProtocolIPv6ICMP, reply[:n])
	if err != nil {
		return err
	}
	switch rm.Type {
	case ipv6.ICMPTypeEchoReply:
		log.Printf("[%s] %s: got reply in %s", addr, resolved.IP, duration)

	case ipv6.ICMPTypeDestinationUnreachable:
		log.Printf("[%s] %s: destination unreachable in %s", addr, resolved.IP, duration)

	default:
		return fmt.Errorf("got %+v from %v; want echo reply", rm, peer)
	}

	return nil
}
