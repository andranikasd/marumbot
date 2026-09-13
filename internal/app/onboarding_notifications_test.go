package app

import (
	"context"
	"errors"
	"testing"
)

type onboardingReminderPreferences struct {
	ReminderStore
	err error
}

func (r onboardingReminderPreferences) UserPreferences(context.Context, string) (UserPreferences, error) {
	off := false
	return UserPreferences{RemindersEnabled: &off}, r.err
}

func TestMiniAppSaveDoesNotSendChatBeforeOptIn(t *testing.T) {
	w := Worker{Reminders: onboardingReminderPreferences{}}
	// No sender or identity resolver is provided: neither may be touched.
	if err := w.OnLoanFiledMessage(t.Context(), "user"); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("preferences unavailable")
	w.Reminders = onboardingReminderPreferences{err: failure}
	if err := w.OnLoanFiledMessage(t.Context(), "user"); !errors.Is(err, failure) {
		t.Fatal("preference outage must not be treated as permission")
	}
}
