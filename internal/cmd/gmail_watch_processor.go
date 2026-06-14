package cmd

import (
	"context"

	"github.com/steipete/gogcli/internal/gmailwatch"
)

func (s *gmailWatchServer) handlePush(ctx context.Context, payload gmailPushPayload) (*gmailHookPayload, error) {
	processor := &gmailwatch.Processor{
		Config: gmailwatch.ProcessorConfig{
			Account:      s.cfg.Account,
			HistoryMax:   s.cfg.HistoryMax,
			ResyncMax:    s.cfg.ResyncMax,
			FetchDelay:   s.cfg.FetchDelay,
			HistoryTypes: s.cfg.HistoryTypes,
			Verbose:      s.cfg.VerboseOutput,
		},
		Repository: s.store,
		NewSource: func(ctx context.Context) (gmailwatch.Source, error) {
			service, err := s.newService(ctx, s.cfg.Account)
			if err != nil {
				return nil, err
			}

			return newGmailWatchSource(service, s.cfg, s.excludeLabelIDs, s.logf), nil
		},
		Now:                 s.currentTime,
		Sleep:               s.sleep,
		IsStaleHistoryError: isStaleHistoryError,
		RateLimitUntil:      gmailWatchRateLimitUntil,
		Logf:                s.logf,
		Warnf:               s.warnf,
	}

	return processor.Handle(ctx, gmailwatch.Notification{
		HistoryID: payload.HistoryID,
		MessageID: payload.MessageID,
	})
}
