package dnsseed

import (
	"context"
	"net"

	"github.com/codeminute-the-dev/lattice/coredns-dnsseed/crawler"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

// LatticeSeeder discovers IP addresses by crawling the Lattice P2P network.
type LatticeSeeder struct {
	Next   plugin.Handler
	Zones  []string
	seeder *crawler.Seeder
	opts   *options
}

func (ps LatticeSeeder) Name() string { return pluginName }

func (ps LatticeSeeder) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	zone := plugin.Zones(ps.Zones).Matches(state.Name())
	if zone == "" {
		return plugin.NextOrFailure(ps.Name(), ps.Next, ctx, w, r)
	}

	maxAnswers := int(ps.opts.maxAnswers)

	var peerIPs []net.IP
	switch state.QType() {
	case dns.TypeA:
		peerIPs = ps.seeder.Addresses(maxAnswers)
	case dns.TypeAAAA:
		peerIPs = ps.seeder.AddressesV6(maxAnswers)
	default:
		return dns.RcodeNotImplemented, nil
	}

	a := new(dns.Msg)
	a.SetReply(r)
	a.Authoritative = true
	a.Answer = make([]dns.RR, 0, maxAnswers)

	for _, ip := range peerIPs {
		var rr dns.RR
		if ip.To4() == nil {
			rr = &dns.AAAA{
				Hdr:  dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeAAAA, Ttl: ps.opts.recordTTL, Class: state.QClass()},
				AAAA: ip,
			}
		} else {
			rr = &dns.A{
				Hdr: dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeA, Ttl: ps.opts.recordTTL, Class: state.QClass()},
				A:   ip,
			}
		}
		a.Answer = append(a.Answer, rr)
	}

	w.WriteMsg(a)
	return dns.RcodeSuccess, nil
}
