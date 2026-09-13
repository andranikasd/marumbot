package app

import "testing"

func TestNotificationTimingRequiresAPair(t *testing.T) {
	lead, minute := 1, 540
	p := UserPreferences{Timezone: "Asia/Yerevan", ReminderLeadDays: &lead}
	if p.Validate() == nil {
		t.Fatal("accepted missing local time")
	}
	p.ReminderLeadDays = nil
	p.ReminderMinute = &minute
	if p.Validate() == nil {
		t.Fatal("accepted missing lead day")
	}
	p.ReminderLeadDays = &lead
	if p.Validate() != nil {
		t.Fatal("valid notification choice rejected")
	}
}
