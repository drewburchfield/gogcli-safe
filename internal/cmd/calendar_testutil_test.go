package cmd

import (
	"context"
	"io"
	"net/http"
	"testing"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func newCalendarServiceForTest(t *testing.T, h http.Handler) (*calendar.Service, func()) {
	t.Helper()

	return newGoogleTestService(t, h, calendar.NewService)
}

func newTestCalendarService(t *testing.T, h http.Handler) (*calendar.Service, func()) {
	t.Helper()
	return newCalendarServiceForTest(t, h)
}

func withCalendarTestService(ctx context.Context, svc *calendar.Service) context.Context {
	return withCalendarTestServiceFactory(ctx, func(context.Context, string) (*calendar.Service, error) {
		return svc, nil
	})
}

func withCalendarTestServiceFactory(ctx context.Context, factory app.CalendarServiceFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime := &app.Runtime{}
	if existing, ok := app.FromContext(ctx); ok {
		*runtime = *existing
	}
	runtime.Services.Calendar = factory
	return app.WithRuntime(ctx, runtime)
}

func executeWithCalendarTestService(t *testing.T, args []string, svc *calendar.Service) executeTestResult {
	t.Helper()
	return executeWithCalendarTestServiceFactory(t, args, func(context.Context, string) (*calendar.Service, error) {
		return svc, nil
	})
}

func executeWithCalendarTestServiceFactory(t *testing.T, args []string, factory app.CalendarServiceFactory) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Calendar: factory,
	}})
}

func newCalendarOutputContext(t *testing.T, stdout, stderr io.Writer) context.Context {
	t.Helper()

	u, err := ui.New(ui.Options{Stdout: stdout, Stderr: stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func newCalendarJSONContext(t *testing.T) context.Context {
	t.Helper()
	return newCalendarJSONOutputContext(t, io.Discard, io.Discard)
}

func newCalendarJSONOutputContext(t *testing.T, stdout, stderr io.Writer) context.Context {
	t.Helper()
	return outfmt.WithMode(newCalendarOutputContext(t, stdout, stderr), outfmt.Mode{JSON: true})
}
