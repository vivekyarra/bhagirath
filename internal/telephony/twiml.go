// Package telephony implements missed-call intake, IVR voice recording and
// outbound verification calls over Twilio (TwiML) and Exotel.
package telephony

import (
	"encoding/xml"
	"strconv"
)

// TwiML is a minimal, escaping-safe TwiML document builder.
type TwiML struct {
	XMLName xml.Name `xml:"Response"`
	Verbs   []any
}

// Say speaks text in a voice/language.
type Say struct {
	XMLName  xml.Name `xml:"Say"`
	Voice    string   `xml:"voice,attr,omitempty"`
	Language string   `xml:"language,attr,omitempty"`
	Text     string   `xml:",chardata"`
}

// Gather collects DTMF digits.
type Gather struct {
	XMLName   xml.Name `xml:"Gather"`
	Action    string   `xml:"action,attr"`
	Method    string   `xml:"method,attr,omitempty"`
	NumDigits int      `xml:"numDigits,attr,omitempty"`
	Timeout   int      `xml:"timeout,attr,omitempty"`
	Says      []Say
}

// Record records the caller.
type Record struct {
	XMLName     xml.Name `xml:"Record"`
	Action      string   `xml:"action,attr"`
	Method      string   `xml:"method,attr,omitempty"`
	MaxLength   int      `xml:"maxLength,attr,omitempty"`
	FinishOnKey string   `xml:"finishOnKey,attr,omitempty"`
	PlayBeep    bool     `xml:"playBeep,attr"`
	Timeout     int      `xml:"timeout,attr,omitempty"`
}

// Redirect jumps to another TwiML URL.
type Redirect struct {
	XMLName xml.Name `xml:"Redirect"`
	Method  string   `xml:"method,attr,omitempty"`
	URL     string   `xml:",chardata"`
}

// Reject declines an inbound call without answering (a free "missed call").
type Reject struct {
	XMLName xml.Name `xml:"Reject"`
	Reason  string   `xml:"reason,attr,omitempty"`
}

// Hangup ends the call.
type Hangup struct {
	XMLName xml.Name `xml:"Hangup"`
}

// Pause waits.
type Pause struct {
	XMLName xml.Name `xml:"Pause"`
	Length  int      `xml:"length,attr"`
}

// Add appends verbs.
func (t *TwiML) Add(v ...any) *TwiML { t.Verbs = append(t.Verbs, v...); return t }

// String renders the document.
func (t *TwiML) String() string {
	b, err := xml.Marshal(t)
	if err != nil {
		return `<?xml version="1.0" encoding="UTF-8"?><Response><Hangup/></Response>`
	}
	return xml.Header + string(b)
}

// Digits parses a single DTMF choice (0 if absent/invalid).
func Digits(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
