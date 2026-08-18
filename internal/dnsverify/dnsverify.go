package dnsverify

import (
	"context"
	"net"
	"strings"
)

type Verifier interface {
	Verify(context.Context, string, string) (bool, error)
}

type System struct {
	Resolver *net.Resolver
}

func (s System) Verify(ctx context.Context, host, token string) (bool, error) {
	resolver := s.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	records, err := resolver.LookupTXT(ctx, "_qh8z."+host)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			return false, nil
		}
		return false, err
	}
	expected := "qh8z-verification=" + token
	for _, record := range records {
		if strings.TrimSpace(record) == expected {
			return true, nil
		}
	}
	return false, nil
}
