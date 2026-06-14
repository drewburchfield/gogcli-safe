package cmd

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/idtoken"

	"github.com/steipete/gogcli/internal/gmailwatch"
)

var errNoNewMessages = gmailwatch.ErrNoNewMessages

const (
	gmailWatchStatusHTTPError = gmailwatch.DeliveryStatusHTTPError
	gmailWatchStatusRateLimit = gmailwatch.DeliveryStatusRateLimit
)

type gmailWatchRateLimitError = gmailwatch.RateLimitError

type gmailWatchServer struct {
	cfg             gmailWatchServeConfig
	store           *gmailWatchStore
	validator       *idtoken.Validator
	newService      func(context.Context, string) (*gmail.Service, error)
	sleep           func(context.Context, time.Duration) error
	hookClient      *http.Client
	excludeLabelIDs map[string]struct{}
	logf            func(string, ...any)
	warnf           func(string, ...any)
	now             func() time.Time
}

func (s *gmailWatchServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !pathMatches(s.cfg.Path, r.URL.Path) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if ok := s.authorize(r); !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	push, err := parsePubSubPush(r)
	if err != nil {
		s.warnf("watch: invalid push payload: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	payload, err := decodeGmailPushPayload(push)
	if err != nil {
		s.warnf("watch: invalid push data: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if payload.EmailAddress != "" && !strings.EqualFold(payload.EmailAddress, s.cfg.Account) {
		s.warnf("watch: ignoring push for %s", payload.EmailAddress)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	processed, err := s.processGmailWatchPayload(r.Context(), payload)
	if err != nil {
		if errors.Is(err, errNoNewMessages) {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var rateErr *gmailWatchRateLimitError
		if errors.As(err, &rateErr) {
			if !rateErr.Until.IsZero() {
				w.Header().Set("Retry-After", retryAfterSeconds(s.currentTime(), rateErr.Until))
			}
			s.warnf("watch: Gmail rate limit circuit open: %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		s.warnf("watch: handle push failed: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if processed == nil || processed.Payload == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if s.cfg.HookURL == "" {
		if s.cfg.AllowNoHook {
			_ = json.NewEncoder(w).Encode(processed.Payload)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *gmailWatchServer) authorize(r *http.Request) bool {
	if s.cfg.VerifyOIDC {
		bearer := bearerToken(r)
		if bearer != "" {
			if ok, err := verifyOIDCToken(r.Context(), s.validator, bearer, s.oidcAudience(r), s.cfg.OIDCEmail); ok {
				return true
			} else if err != nil {
				s.warnf("watch: oidc verify failed: %v", err)
			}
		}
		if s.cfg.SharedToken != "" {
			return sharedTokenMatches(r, s.cfg.SharedToken)
		}
		return false
	}
	if s.cfg.SharedToken == "" {
		return true
	}
	return sharedTokenMatches(r, s.cfg.SharedToken)
}

func (s *gmailWatchServer) oidcAudience(r *http.Request) string {
	if s.cfg.OIDCAudience != "" {
		return s.cfg.OIDCAudience
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		parts := strings.Split(xf, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			scheme = strings.TrimSpace(parts[0])
		}
	}
	host := r.Host
	if xf := r.Header.Get("X-Forwarded-Host"); xf != "" {
		parts := strings.Split(xf, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			host = strings.TrimSpace(parts[0])
		}
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, r.URL.Path)
}

func (s *gmailWatchServer) sendHook(ctx context.Context, payload *gmailHookPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.HookURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.HookToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.HookToken)
	}
	resp, err := s.hookClient.Do(req)
	if err != nil {
		_ = s.store.RecordDelivery(gmailwatch.DeliveryStatusError, err.Error(), s.currentTime())

		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = s.store.RecordDelivery(
			gmailwatch.DeliveryStatusHTTPError,
			fmt.Sprintf("status %d", resp.StatusCode),
			s.currentTime(),
		)

		return fmt.Errorf("hook status %d", resp.StatusCode)
	}
	_ = s.store.RecordDelivery(gmailwatch.DeliveryStatusOK, "", s.currentTime())

	return nil
}

func parsePubSubPush(r *http.Request) (*pubsubPushEnvelope, error) {
	defer r.Body.Close()
	limit := int64(defaultPushBodyLimitBytes)
	data, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("push body too large")
	}
	var envelope pubsubPushEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	if envelope.Message.Data == "" {
		return nil, errors.New("missing message.data")
	}
	return &envelope, nil
}

func decodeGmailPushPayload(envelope *pubsubPushEnvelope) (gmailPushPayload, error) {
	decoded, err := base64.StdEncoding.DecodeString(envelope.Message.Data)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(envelope.Message.Data)
		if err != nil {
			return gmailPushPayload{}, err
		}
	}
	var payload gmailPushPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return gmailPushPayload{}, err
	}
	payload.MessageID = strings.TrimSpace(envelope.Message.MessageID)
	return payload, nil
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func sharedTokenMatches(r *http.Request, expected string) bool {
	if expected == "" {
		return false
	}
	token := r.Header.Get("x-gog-token")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func verifyOIDCToken(ctx context.Context, validator *idtoken.Validator, token, audience, expectedEmail string) (bool, error) {
	if validator == nil {
		return false, errors.New("no OIDC validator")
	}
	payload, err := validator.Validate(ctx, token, audience)
	if err != nil {
		return false, err
	}
	if expectedEmail == "" {
		return true, nil
	}
	email, _ := payload.Claims["email"].(string)
	if !strings.EqualFold(email, expectedEmail) {
		return false, fmt.Errorf("oidc email mismatch: %s", email)
	}
	return true, nil
}

func pathMatches(expected, actual string) bool {
	if expected == actual {
		return true
	}
	if strings.HasSuffix(expected, "/") {
		return strings.HasPrefix(actual, expected)
	}
	return strings.HasPrefix(actual, expected+"/")
}

func (s *gmailWatchServer) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}

	return time.Now()
}

func isStaleHistoryError(err error) bool {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		if gerr.Code == http.StatusBadRequest || gerr.Code == http.StatusNotFound {
			msg := strings.ToLower(gerr.Message)
			if strings.Contains(msg, "history") {
				return true
			}
			for _, item := range gerr.Errors {
				if strings.Contains(strings.ToLower(item.Message), "history") {
					return true
				}
				if gerr.Code == http.StatusNotFound && strings.EqualFold(strings.TrimSpace(item.Reason), "notfound") {
					return true
				}
			}
			if gerr.Code == http.StatusNotFound && strings.Contains(msg, "not found") {
				return true
			}
		}
	}
	return strings.Contains(strings.ToLower(err.Error()), "history")
}

func isNotFoundAPIError(err error) bool {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		return gerr.Code == http.StatusNotFound
	}
	return false
}

func gmailWatchRateLimitUntil(err error, now time.Time) (time.Time, bool) {
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) || gerr.Code != http.StatusTooManyRequests {
		return time.Time{}, false
	}
	if until, ok := parseRetryAfterUntil(gerr.Header.Get("Retry-After"), now); ok {
		return until, true
	}
	return now.Add(time.Minute), true
}

func parseRetryAfterUntil(raw string, now time.Time) (time.Time, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false
	}
	if seconds, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		if seconds < 0 {
			seconds = 0
		}
		return now.Add(time.Duration(seconds) * time.Second), true
	}
	if parsed, err := http.ParseTime(trimmed); err == nil {
		return parsed, true
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed, true
	}
	return time.Time{}, false
}

func retryAfterSeconds(now, until time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	seconds := int64(until.Sub(now).Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return strconv.FormatInt(seconds, 10)
}
