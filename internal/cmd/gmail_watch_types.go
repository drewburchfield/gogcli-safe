package cmd

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/gmailwatch"
)

const (
	defaultWatchPath             = "/gmail-pubsub"
	defaultWatchPort             = 8788
	defaultHookMaxBytes          = 20000
	defaultHistoryMaxResults     = 100
	defaultHistoryResyncMax      = 10
	defaultHistoryFetchDelay     = 3 * time.Second
	defaultPushBodyLimitBytes    = 1024 * 1024
	defaultHookRequestTimeoutSec = 10
)

type gmailWatchServeConfig struct {
	Account       string
	Bind          string
	Port          int
	Path          string
	VerifyOIDC    bool
	OIDCEmail     string
	OIDCAudience  string
	SharedToken   string
	HookURL       string
	HookToken     string
	IncludeBody   bool
	MaxBodyBytes  int
	ExcludeLabels []string
	HistoryMax    int64
	ResyncMax     int64
	FetchDelay    time.Duration
	HistoryTypes  []string
	HookTimeout   time.Duration
	DateLocation  *time.Location
	PersistHook   bool
	AllowNoHook   bool
	VerboseOutput bool
}

var gmailHistoryTypes = []string{
	"messageAdded",
	"messageDeleted",
	"labelAdded",
	"labelRemoved",
}

var gmailHistoryTypesHelp = strings.Join(gmailHistoryTypes, ",")

var gmailHistoryTypeAliases = func() map[string]string {
	aliases := make(map[string]string, len(gmailHistoryTypes)+4)
	for _, historyType := range gmailHistoryTypes {
		aliases[strings.ToLower(historyType)] = historyType
	}
	aliases["messagesadded"] = "messageAdded"
	aliases["messagesdeleted"] = "messageDeleted"
	aliases["labelsadded"] = "labelAdded"
	aliases["labelsremoved"] = "labelRemoved"
	return aliases
}()

func parseHistoryTypes(values []string) ([]string, error) {
	if len(values) == 0 {
		// Default to messageAdded for backward compatibility.
		// Previously this was hardcoded; returning nil would fetch ALL types.
		return []string{"messageAdded"}, nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			normalized, ok := gmailHistoryTypeAliases[strings.ToLower(trimmed)]
			if !ok {
				return nil, usage("--history-types must be one of " + gmailHistoryTypesHelp)
			}
			if _, exists := seen[normalized]; exists {
				continue
			}
			seen[normalized] = struct{}{}
			out = append(out, normalized)
		}
	}
	if len(out) == 0 {
		return nil, usage("--history-types must include at least one value")
	}
	return out, nil
}

type pubsubPushEnvelope struct {
	Message struct {
		Data        string            `json:"data"`
		MessageID   string            `json:"messageId"`
		PublishTime string            `json:"publishTime"`
		Attributes  map[string]string `json:"attributes"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

type gmailPushPayload struct {
	EmailAddress string `json:"emailAddress"`
	HistoryID    string `json:"historyId"`
	MessageID    string `json:"-"`
}

func (p *gmailPushPayload) UnmarshalJSON(data []byte) error {
	var raw struct {
		EmailAddress string          `json:"emailAddress"`
		HistoryID    json.RawMessage `json:"historyId"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.EmailAddress = raw.EmailAddress
	if len(raw.HistoryID) == 0 {
		p.HistoryID = ""
		return nil
	}
	var asString string
	if err := json.Unmarshal(raw.HistoryID, &asString); err == nil {
		p.HistoryID = asString
		return nil
	}
	var asNumber json.Number
	if err := json.Unmarshal(raw.HistoryID, &asNumber); err == nil {
		if v := strings.TrimSpace(asNumber.String()); v != "" {
			p.HistoryID = v
			return nil
		}
	}
	return usage("historyId must be string or number")
}

type gmailHookPayload = gmailwatch.Payload
