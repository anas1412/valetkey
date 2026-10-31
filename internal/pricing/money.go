// Package pricing turns token usage into money: live provider prices, peak and
// off-peak windows, cache and reasoning rates, all in exact integer maths.
package pricing

import (
	"fmt"
	"math"
	"math/big"
	"strings"
)

// USD is an amount of money in pico-dollars (1e-12 USD). Integers keep sums exact;
// rounding happens only when an amount is shown.
type USD int64

const Dollar USD = 1_000_000_000_000

// ParseUSD reads a decimal dollar amount such as "20", "0.50" or "0.000000003".
func ParseUSD(s string) (USD, error) {
	r, ok := new(big.Rat).SetString(strings.TrimPrefix(strings.TrimSpace(s), "$"))
	if !ok || r.Sign() < 0 {
		return 0, fmt.Errorf("invalid dollar amount %q", s)
	}
	r.Mul(r, new(big.Rat).SetInt64(int64(Dollar)))
	if !r.IsInt() {
		// Below a pico-dollar: round half up.
		r.Add(r, big.NewRat(1, 2))
	}
	n := new(big.Int).Quo(r.Num(), r.Denom())
	if !n.IsInt64() {
		return 0, fmt.Errorf("dollar amount %q is too large", s)
	}
	return USD(n.Int64()), nil
}

// PerMillion converts a catalogue price in dollars per million tokens into USD per token.
func PerMillion(dollars float64) USD {
	return USD(math.Round(dollars * 1e6))
}

// PerMillionString is PerMillion for exact decimal strings (CRD overrides).
func PerMillionString(s string) (USD, error) {
	u, err := ParseUSD(s)
	if err != nil {
		return 0, err
	}
	return (u + 500_000) / 1_000_000, nil
}

// String shows dollars with up to 6 decimals, e.g. "$12.3401".
func (u USD) String() string {
	neg := u < 0
	if neg {
		u = -u
	}
	micro := (u + 500_000) / 1_000_000
	s := fmt.Sprintf("%d.%06d", micro/1_000_000, micro%1_000_000)
	s = strings.TrimRight(s, "0")
	if strings.HasSuffix(s, ".") {
		s += "00"
	} else if i := strings.IndexByte(s, '.'); len(s)-i == 2 {
		s += "0"
	}
	if neg {
		return "-$" + s
	}
	return "$" + s
}

// Decimal is the exact dollar amount as a plain decimal string ("12.000000340001"),
// used in CRD status so nothing is lost across restarts.
func (u USD) Decimal() string {
	s := fmt.Sprintf("%d.%012d", u/Dollar, u%Dollar)
	return strings.TrimRight(strings.TrimRight(s, "0"), ".")
}

// Float is for Prometheus metrics only.
func (u USD) Float() float64 { return float64(u) / float64(Dollar) }
