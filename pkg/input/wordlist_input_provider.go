package input

import (
	"fmt"
	"strings"

	"github.com/MaltsevaNata/ffuf/v3/pkg/ffuf"
	"github.com/ffuf/pencode/pkg/pencode"
)

type WordlistInputProvider struct {
	Providers   []ffuf.InternalInputProvider
	Encoders    map[string]*pencode.Chain
	Config      *ffuf.Config
	position    int
	msbIterator int
	wl          *WordlistInput
}

func NewWordlistInputProvider(conf *ffuf.Config, wl *WordlistInput) (ffuf.InputProvider, ffuf.Multierror) {
	errs := ffuf.NewMultierror()
	ip := WordlistInputProvider{Config: conf, msbIterator: 0, Encoders: make(map[string]*pencode.Chain), wl: wl}
	// Initialize the correct inputprovider
	for _, v := range conf.InputProviders {
		err := ip.AddProvider(v)
		if err != nil {
			errs.Add(err)
		}
	}
	return &ip, errs
}

func (i *WordlistInputProvider) AddProvider(provider ffuf.InputProviderConfig) error {
	// Default to wordlist
	i.Providers = append(i.Providers, i.wl)

	if len(provider.Encoders) > 0 {
		chain := pencode.NewChain()
		err := chain.Initialize(strings.Split(strings.TrimSpace(provider.Encoders), " "))
		if err != nil {
			return err
		}
		i.Encoders[provider.Keyword] = chain
	}
	return nil
}

// ActivateKeywords enables / disables wordlists based on list of active keywords
func (i *WordlistInputProvider) ActivateKeywords(kws []string) {
	for _, p := range i.Providers {
		if ffuf.StrInSlice(p.Keyword(), kws) {
			p.Active()
		} else {
			p.Disable()
		}
	}
}

// Position will return the current position of progress
func (i *WordlistInputProvider) Position() int {
	return i.position
}

// SetPosition will reset the MainInputProvider to a specific position
func (i *WordlistInputProvider) SetPosition(pos int) {
	if i.Config.InputMode == "clusterbomb" || i.Config.InputMode == "sniper" {
		i.setclusterbombPosition(pos)
	} else {
		i.setpitchforkPosition(pos)
	}
}

// Keywords returns a slice of all keywords in the inputprovider
func (i *WordlistInputProvider) Keywords() []string {
	kws := make([]string, 0)
	for _, p := range i.Providers {
		kws = append(kws, p.Keyword())
	}
	return kws
}

// Next will increment the cursor position, and return a boolean telling if there's inputs left
func (i *WordlistInputProvider) Next() bool {
	if i.position >= i.Total() {
		return false
	}
	i.position++
	return true
}

// Value returns a map of inputs for keywords
func (i *WordlistInputProvider) Value() map[string][]byte {
	retval := make(map[string][]byte)
	if i.Config.InputMode == "clusterbomb" || i.Config.InputMode == "sniper" {
		retval = i.clusterbombValue()
	}
	if i.Config.InputMode == "pitchfork" {
		retval = i.pitchforkValue()
	}
	if len(i.Encoders) > 0 {
		for key, val := range retval {
			chain, ok := i.Encoders[key]
			if ok {
				tmpVal, err := chain.Encode([]byte(val))
				if err != nil {
					fmt.Printf("ERROR: %s\n", err)
				}
				retval[key] = tmpVal
			}
		}
	}
	return retval
}

// Reset resets all the inputproviders and counters
func (i *WordlistInputProvider) Reset() {
	for _, p := range i.Providers {
		p.ResetPosition()
	}
	i.position = 0
	i.msbIterator = 0
}

// pitchforkValue returns a map of keyword:value pairs including all inputs.
// This mode will iterate through wordlists in lockstep.
func (i *WordlistInputProvider) pitchforkValue() map[string][]byte {
	values := make(map[string][]byte)
	for _, p := range i.Providers {
		if !p.Active() {
			// The inputprovider is disabled
			continue
		}
		if !p.Next() {
			// Loop to beginning if the inputprovider has been exhausted
			p.ResetPosition()
		}
		values[p.Keyword()] = p.Value()
		p.IncrementPosition()
	}
	return values
}

func (i *WordlistInputProvider) setpitchforkPosition(pos int) {
	for _, p := range i.Providers {
		p.SetPosition(pos)
	}
}

// clusterbombValue returns map of keyword:value pairs including all inputs.
// this mode will iterate through all possible combinations.
func (i *WordlistInputProvider) clusterbombValue() map[string][]byte {
	values := make(map[string][]byte)
	// Should we signal the next InputProvider in the slice to increment
	signalNext := false
	first := true
	index := 0
	for _, p := range i.Providers {
		if !p.Active() {
			continue
		}
		if signalNext {
			p.IncrementPosition()
			signalNext = false
		}
		if !p.Next() {
			// No more inputs in this inputprovider
			if index == i.msbIterator {
				// Reset all previous wordlists and increment the msb counter
				i.msbIterator += 1
				i.clusterbombIteratorReset()
				// Start again
				return i.clusterbombValue()
			}
			p.ResetPosition()
			signalNext = true
		}
		values[p.Keyword()] = p.Value()
		if first {
			p.IncrementPosition()
			first = false
		}
		index += 1
	}
	return values
}

func (i *WordlistInputProvider) setclusterbombPosition(pos int) {
	i.Reset()
	if pos > i.Total() {
		// noop
		return
	}
	for i.position < pos-1 {
		i.Next()
		i.Value()
	}
}

func (i *WordlistInputProvider) clusterbombIteratorReset() {
	index := 0
	for _, p := range i.Providers {
		if !p.Active() {
			continue
		}
		if index < i.msbIterator {
			p.ResetPosition()
		}
		if index == i.msbIterator {
			p.IncrementPosition()
		}
		index += 1
	}
}

// Total returns the amount of input combinations available
func (i *WordlistInputProvider) Total() int {
	count := 0
	if i.Config.InputMode == "pitchfork" {
		for _, p := range i.Providers {
			if !p.Active() {
				continue
			}
			if p.Total() > count {
				count = p.Total()
			}
		}
	}
	if i.Config.InputMode == "clusterbomb" || i.Config.InputMode == "sniper" {
		count = 1
		for _, p := range i.Providers {
			if !p.Active() {
				continue
			}
			count = count * p.Total()
		}
	}
	return count
}