package cmd

import (
	"strings"

	"google.golang.org/api/calendar/v3"
)

type focusTimeInput struct {
	AutoDecline    string
	DeclineMessage string
	ChatStatus     string
}

type outOfOfficeInput struct {
	AutoDecline            string
	DeclineMessage         string
	DeclineMessageProvided bool
}

type calendarCreatePlan struct {
	CalendarID  string
	SendUpdates string
	WithMeet    bool
	WithZoom    bool
	Event       *calendar.Event
}

type calendarUpdateFields struct {
	Summary               bool
	Description           bool
	Location              bool
	LocationSearch        bool
	PlaceID               bool
	From                  bool
	To                    bool
	StartTimezone         bool
	EndTimezone           bool
	AllDay                bool
	Attendees             bool
	AddAttendee           bool
	Attachments           bool
	Recurrence            bool
	Reminders             bool
	ColorID               bool
	Visibility            bool
	Transparency          bool
	GuestsCanInviteOthers bool
	GuestsCanModify       bool
	GuestsCanSeeOthers    bool
	WithMeet              bool
	RegenerateMeet        bool
	WithZoom              bool
	RegenerateZoom        bool
	RemoveZoom            bool
	PrivateProps          bool
	SharedProps           bool
	FocusAutoDecline      bool
	FocusDeclineMessage   bool
	FocusChatStatus       bool
	OOOAutoDecline        bool
	OOODeclineMessage     bool
	WorkingLocationType   bool
	WorkingOfficeLabel    bool
	WorkingBuildingID     bool
	WorkingFloorID        bool
	WorkingDeskID         bool
	WorkingCustomLabel    bool
}

func (f calendarUpdateFields) focusEventType() bool {
	return f.FocusAutoDecline || f.FocusDeclineMessage || f.FocusChatStatus
}

func (f calendarUpdateFields) outOfOfficeEventType() bool {
	return f.OOOAutoDecline || f.OOODeclineMessage
}

func (f calendarUpdateFields) workingLocationEventType() bool {
	return f.WorkingLocationType ||
		f.WorkingOfficeLabel ||
		f.WorkingBuildingID ||
		f.WorkingFloorID ||
		f.WorkingDeskID ||
		f.WorkingCustomLabel
}

func (f calendarUpdateFields) zoomMutation() bool {
	return f.WithZoom || f.RegenerateZoom || f.RemoveZoom
}

func buildCalendarCreatePlan(c *CalendarCreateCmd) (*calendarCreatePlan, error) {
	eventType, err := c.resolveCreateEventType()
	if err != nil {
		return nil, err
	}

	summary := strings.TrimSpace(c.Summary)
	if summary == "" {
		summary = c.defaultSummaryForEventType(eventType)
	}
	if summary == "" || strings.TrimSpace(c.From) == "" || strings.TrimSpace(c.To) == "" {
		return nil, usage("required: --summary, --from, --to")
	}

	colorID, err := validateColorId(c.ColorId)
	if err != nil {
		return nil, err
	}
	visibility, err := validateVisibility(c.Visibility)
	if err != nil {
		return nil, err
	}
	transparency, err := validateTransparency(c.Transparency)
	if err != nil {
		return nil, err
	}
	sendUpdates, err := validateSendUpdates(c.SendUpdates)
	if err != nil {
		return nil, err
	}
	reminders, err := buildReminders(c.Reminders)
	if err != nil {
		return nil, err
	}
	allDay, err := resolveCreateAllDay(c.From, c.To, c.AllDay, eventType)
	if err != nil {
		return nil, err
	}
	start, err := buildEventDateTimeWithTimezone(c.From, allDay, c.StartTimezone, "--start-timezone")
	if err != nil {
		return nil, err
	}
	end, err := buildEventDateTimeWithTimezone(c.To, allDay, c.EndTimezone, "--end-timezone")
	if err != nil {
		return nil, err
	}

	event := &calendar.Event{
		Summary:            summary,
		Description:        strings.TrimSpace(c.Description),
		Location:           strings.TrimSpace(c.Location),
		Start:              start,
		End:                end,
		Attendees:          buildAttendees(c.Attendees),
		Recurrence:         buildRecurrence(c.Recurrence),
		Reminders:          reminders,
		ColorId:            colorID,
		Visibility:         applyEventTypeVisibilityDefault(visibility, eventType),
		Transparency:       applyEventTypeTransparencyDefault(transparency, eventType),
		ConferenceData:     buildConferenceData(conferenceChoice{provider: conferenceProvider(c.WithMeet, c.WithZoom)}),
		Attachments:        buildAttachments(c.Attachments),
		ExtendedProperties: buildExtendedProperties(c.PrivateProps, c.SharedProps),
	}
	if c.GuestsCanInviteOthers != nil {
		event.GuestsCanInviteOthers = c.GuestsCanInviteOthers
	}
	if c.GuestsCanModify != nil {
		event.GuestsCanModify = *c.GuestsCanModify
	}
	if c.GuestsCanSeeOthers != nil {
		event.GuestsCanSeeOtherGuests = c.GuestsCanSeeOthers
	}
	if strings.TrimSpace(c.SourceUrl) != "" {
		event.Source = &calendar.EventSource{
			Url:   strings.TrimSpace(c.SourceUrl),
			Title: strings.TrimSpace(c.SourceTitle),
		}
	}
	if c.resolvedPlace != nil {
		event.Location = formatCalendarPlaceLocation(c.resolvedPlace)
		applyCalendarPlaceProperties(event, c.resolvedPlace)
	}

	if err := c.applyCreateEventType(event, eventType); err != nil {
		return nil, err
	}

	return &calendarCreatePlan{
		CalendarID:  strings.TrimSpace(c.CalendarID),
		SendUpdates: sendUpdates,
		WithMeet:    c.WithMeet,
		WithZoom:    c.WithZoom,
		Event:       event,
	}, nil
}

func conferenceProvider(withMeet, withZoom bool) string {
	switch {
	case withMeet:
		return conferenceProviderMeet
	case withZoom:
		return conferenceProviderZoom
	default:
		return ""
	}
}

func buildFocusTimeProperties(input focusTimeInput) (*calendar.EventFocusTimeProperties, error) {
	autoDecline := strings.TrimSpace(input.AutoDecline)
	if autoDecline == "" {
		autoDecline = defaultFocusAutoDecline
	}
	autoDeclineMode, err := validateAutoDeclineMode(autoDecline)
	if err != nil {
		return nil, err
	}

	chatStatus := strings.TrimSpace(input.ChatStatus)
	if chatStatus == "" {
		chatStatus = defaultFocusChatStatus
	}
	chatStatusValue, err := validateChatStatus(chatStatus)
	if err != nil {
		return nil, err
	}

	return &calendar.EventFocusTimeProperties{
		AutoDeclineMode: autoDeclineMode,
		DeclineMessage:  strings.TrimSpace(input.DeclineMessage),
		ChatStatus:      chatStatusValue,
	}, nil
}

func buildOutOfOfficeProperties(input outOfOfficeInput) (*calendar.EventOutOfOfficeProperties, error) {
	autoDecline := strings.TrimSpace(input.AutoDecline)
	if autoDecline == "" {
		autoDecline = defaultOOOAutoDecline
	}
	autoDeclineMode, err := validateAutoDeclineMode(autoDecline)
	if err != nil {
		return nil, err
	}

	declineMessage := strings.TrimSpace(input.DeclineMessage)
	if declineMessage == "" && !input.DeclineMessageProvided {
		declineMessage = defaultOOODeclineMsg
	}

	return &calendar.EventOutOfOfficeProperties{
		AutoDeclineMode: autoDeclineMode,
		DeclineMessage:  declineMessage,
	}, nil
}
