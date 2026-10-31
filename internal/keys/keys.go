// Package keys mints and hashes virtual keys, compares secrets in constant time,
// signs session cookies, and matches client IPs against allow lists.
package keys

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

const prefix = "sk-proxy-"

// Generate returns sk-proxy- + 32 chars of base64url entropy.
func Generate() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b)
}

// Hash is the SHA-256 hex digest stored in ProxyKey status, never the key itself.
func Hash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// Equal compares two secrets of any length in constant time.
func Equal(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// Sign returns an HMAC-SHA256 hex signature of value.
func Sign(secret, value string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(value))
	return hex.EncodeToString(m.Sum(nil))
}

// AllowList is a set of IPv4/IPv6 prefixes.
type AllowList []netip.Prefix

// ParseAllowList accepts CIDRs or bare addresses (single host). It fails on any invalid entry.
func ParseAllowList(subnets []string) (AllowList, error) {
	list := make(AllowList, 0, len(subnets))
	for _, s := range subnets {
		s = strings.TrimSpace(s)
		if !strings.Contains(s, "/") {
			addr, err := netip.ParseAddr(s)
			if err != nil {
				return nil, fmt.Errorf("invalid subnet %q", s)
			}
			s += "/" + strconv.Itoa(addr.BitLen())
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("invalid subnet %q", s)
		}
		list = append(list, p.Masked())
	}
	return list, nil
}

// Allows reports whether ip is in the list. IPv4-mapped IPv6 (::ffff:10.0.0.1) matches as IPv4.
func (l AllowList) Allows(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, p := range l {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
