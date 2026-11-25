package app

import (
	"testing"
	"time"
)

func TestQuotaServiceTimezoneReset(t *testing.T) {
	svc := NewQuotaService(nil, QuotaConfig{Timezone: "Asia/Shanghai"})
	// 2026-11-25 16:30 UTC is 2026-11-26 00:30 in Shanghai.
	now := time.Date(2026, 11, 25, 16, 30, 0, 0, time.UTC)
	day := svc.Day(now)
	if day.Format("2006-01-02") != "2026-11-26" || day.Hour() != 0 {
		t.Fatalf("day=%v", day)
	}
	next := svc.NextReset(now)
	if want := time.Date(2026, 11, 27, 0, 0, 0, 0, svc.Loc); !next.Equal(want) {
		t.Fatalf("next=%v want=%v", next, want)
	}
}

func TestQuotaServiceInvalidTimezoneFallsBackUTC(t *testing.T) {
	svc := NewQuotaService(nil, QuotaConfig{Timezone: "Not/AZone"})
	if svc.Loc != time.UTC {
		t.Fatalf("loc=%v", svc.Loc)
	}
}
