package pricing

import (
	"slices"
	"time"
)

// Rates are prices per token for each kind of token a provider bills.
type Rates struct {
	Input        USD `json:"input"`
	Output       USD `json:"output"`
	CacheRead    USD `json:"cacheRead"`
	CacheWrite5m USD `json:"cacheWrite5m"`
	CacheWrite1h USD `json:"cacheWrite1h"`
	Reasoning    USD `json:"reasoning,omitempty"`   // 0 = same as Output
	InputAudio   USD `json:"inputAudio,omitempty"`  // 0 = same as Input
	OutputAudio  USD `json:"outputAudio,omitempty"` // 0 = same as Output
}

// Tokens is one request's usage, split the way providers bill it. Reasoning is part
// of Output; AudioInput is part of Input and AudioOutput part of Output.
type Tokens struct {
	Input, CacheRead, CacheWrite5m, CacheWrite1h, Output, Reasoning int
	AudioInput, AudioOutput                                         int
}

// Cost prices a request. Reasoning and audio tokens are priced differently only
// when the provider lists a separate rate for them.
func (r Rates) Cost(t Tokens) USD {
	c := r.CacheRead*USD(t.CacheRead) + r.CacheWrite5m*USD(t.CacheWrite5m) + r.CacheWrite1h*USD(t.CacheWrite1h)
	text := t.Input
	if r.InputAudio != 0 {
		audio := min(t.AudioInput, t.Input)
		c += r.InputAudio * USD(audio)
		text -= audio
	}
	c += r.Input * USD(text)

	out := t.Output
	if r.OutputAudio != 0 {
		audio := min(t.AudioOutput, out)
		c += r.OutputAudio * USD(audio)
		out -= audio
	}
	if r.Reasoning != 0 && r.Reasoning != r.Output {
		reasoning := min(t.Reasoning, out)
		c += r.Reasoning * USD(reasoning)
		out -= reasoning
	}
	return c + r.Output*USD(out)
}

// MaxInput is the highest price any input token can cost.
func (r Rates) MaxInput() USD {
	return max(r.Input, r.CacheRead, r.CacheWrite5m, r.CacheWrite1h, r.InputAudio)
}

// MaxOutput is the highest price any output token can cost.
func (r Rates) MaxOutput() USD { return max(r.Output, r.Reasoning, r.OutputAudio) }

// Tier applies when a request's input is above a size (long-context pricing).
type Tier struct {
	Above int   `json:"above"`
	Rates Rates `json:"rates"`
}

// Window is a time-of-day price (DeepSeek peak hours). Start and End are minutes
// after midnight UTC; End may be 1440.
type Window struct {
	Days  []time.Weekday `json:"days"`
	Start int            `json:"start"`
	End   int            `json:"end"`
	Rates Rates          `json:"rates"`
}

func (w Window) contains(t time.Time) bool {
	t = t.UTC()
	m := t.Hour()*60 + t.Minute()
	return slices.Contains(w.Days, t.Weekday()) && m >= w.Start && m < w.End
}

// Price is everything known about one model's price.
type Price struct {
	Rates   Rates    `json:"rates"`             // the normal (off-peak) price
	Tiers   []Tier   `json:"tiers,omitempty"`   // sorted by Above
	Windows []Window `json:"windows,omitempty"` // peak windows; outside them Rates apply
	// Holidays names a calendar ("CN") whose holidays are billed at Rates all day.
	Holidays  string `json:"holidays,omitempty"`
	MaxOutput int    `json:"maxOutput,omitempty"`
	Source    string `json:"source"`
	Warning   string `json:"warning,omitempty"`
}

// Period says whether a request at t falls in a peak window.
const (
	Peak    = "peak"
	OffPeak = "off-peak"
)

// At returns the rates for a request that starts at t with the given input size,
// and "peak"/"off-peak" for models with time windows ("" otherwise).
func (p Price) At(t time.Time, inputTokens int, cal Calendars) (Rates, string) {
	for i := len(p.Tiers) - 1; i >= 0; i-- {
		if inputTokens > p.Tiers[i].Above {
			return p.Tiers[i].Rates, ""
		}
	}
	if len(p.Windows) == 0 {
		return p.Rates, ""
	}
	if p.Holidays != "" && cal.IsHoliday(p.Holidays, t) {
		return p.Rates, OffPeak
	}
	for _, w := range p.Windows {
		if w.contains(t) {
			return w.Rates, Peak
		}
	}
	return p.Rates, OffPeak
}

// Worst is the most a request can cost at t: input tokens at the dearest input
// rate and output tokens at the dearest output rate, including long-context tiers.
func (p Price) Worst(t time.Time, inputTokens, outputTokens int, cal Calendars) USD {
	r, _ := p.At(t, inputTokens, cal)
	return r.MaxInput()*USD(inputTokens) + r.MaxOutput()*USD(outputTokens)
}

// NextChange is when the price of a windowed model next switches between peak
// and off-peak after t (zero when it has no windows). Holidays are ignored.
func (p Price) NextChange(t time.Time) time.Time {
	if len(p.Windows) == 0 {
		return time.Time{}
	}
	t = t.UTC().Truncate(time.Minute)
	in := func(x time.Time) bool {
		return slices.ContainsFunc(p.Windows, func(w Window) bool { return w.contains(x) })
	}
	now := in(t)
	for x := t.Add(time.Minute); x.Before(t.Add(8 * 24 * time.Hour)); x = x.Add(time.Minute) {
		if in(x) != now {
			return x
		}
	}
	return time.Time{}
}
