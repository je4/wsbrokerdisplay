package ntp

import (
	"context"
	"testing"
	"time"

	"emperror.dev/errors"
	"github.com/beevik/ntp"
)

type mockNTPClient struct {
	response *ntp.Response
	err      error
}

func (m *mockNTPClient) NTP(ctx context.Context, opts ...ntp.QueryOptions) (*ntp.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func TestNewServiceDefaults(t *testing.T) {
	svc := New(nil, "", 0, 0, nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}

	if svc.server != DefaultServer {
		t.Errorf("expected default server %s, got %s", DefaultServer, svc.server)
	}
	if svc.interval != DefaultInterval {
		t.Errorf("expected default interval %v, got %v", DefaultInterval, svc.interval)
	}
	if svc.timeout != DefaultTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultTimeout, svc.timeout)
	}

	st := svc.Status()
	if st.Synced {
		t.Errorf("expected initial status Synced=false, got true")
	}
	if st.Server != DefaultServer {
		t.Errorf("expected server %s in status, got %s", DefaultServer, st.Server)
	}
}

func TestFormatReferenceID(t *testing.T) {
	// Stratum 1 ASCII identifier
	gpsID := uint32(('G' << 24) | ('P' << 16) | ('S' << 8) | '0')
	refStr := formatReferenceID(gpsID, 1)
	if refStr != "GPS0" {
		t.Errorf("expected GPS0, got %s", refStr)
	}

	// Stratum 2 IP address identifier
	ipID := uint32((192 << 24) | (168 << 16) | (1 << 8) | 100)
	ipStr := formatReferenceID(ipID, 2)
	if ipStr != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got %s", ipStr)
	}
}

func TestFormatLeapIndicator(t *testing.T) {
	if formatLeapIndicator(ntp.LeapNoWarning) != "no warning" {
		t.Errorf("unexpected leap string for LeapNoWarning: %s", formatLeapIndicator(ntp.LeapNoWarning))
	}
	if formatLeapIndicator(ntp.LeapAddSecond) != "last minute has 61 seconds" {
		t.Errorf("unexpected leap string for LeapAddSecond: %s", formatLeapIndicator(ntp.LeapAddSecond))
	}
	if formatLeapIndicator(ntp.LeapDelSecond) != "last minute has 59 seconds" {
		t.Errorf("unexpected leap string for LeapDelSecond: %s", formatLeapIndicator(ntp.LeapDelSecond))
	}
	if formatLeapIndicator(ntp.LeapNotInSync) != "clock not synchronized" {
		t.Errorf("unexpected leap string for LeapNotInSync: %s", formatLeapIndicator(ntp.LeapNotInSync))
	}
}

func TestMockClientSyncSuccess(t *testing.T) {
	now := time.Now().UTC()
	mockClient := &mockNTPClient{
		response: &ntp.Response{
			Time:          now,
			ReferenceTime: now,
			ClockOffset:   2500 * time.Microsecond,
			RTT:           15 * time.Millisecond,
			Stratum:       2,
			ReferenceID:   uint32((10 << 24) | (0 << 16) | (0 << 8) | 1),
			Precision:     time.Microsecond,
			Leap:          ntp.LeapNoWarning,
		},
	}

	svc := New(mockClient, "wsbroker", 10*time.Minute, 5*time.Second, nil)
	err := svc.Sync()
	if err != nil {
		t.Fatalf("expected successful sync with mock client, got: %v", err)
	}

	st := svc.Status()
	if !st.Synced {
		t.Errorf("expected Synced=true")
	}
	if st.Server != "wsbroker" {
		t.Errorf("expected server 'wsbroker', got '%s'", st.Server)
	}
	if st.Stratum != 2 {
		t.Errorf("expected stratum 2, got %d", st.Stratum)
	}
	if st.Offset != 2500*time.Microsecond {
		t.Errorf("expected offset 2.5ms, got %v", st.Offset)
	}
	if st.Error != "" {
		t.Errorf("expected empty error, got '%s'", st.Error)
	}
}

func TestMockClientSyncError(t *testing.T) {
	mockClient := &mockNTPClient{
		err: errors.New("client is not connected"),
	}

	svc := New(mockClient, "wsbroker", 10*time.Minute, 5*time.Second, nil)
	err := svc.Sync()
	if err == nil {
		t.Fatal("expected error with disconnected client, got nil")
	}

	st := svc.Status()
	if st.Synced {
		t.Errorf("expected Synced=false")
	}
	if st.Error == "" {
		t.Errorf("expected non-empty error")
	}
}

func TestUnreachableServerHandling(t *testing.T) {
	svc := New(nil, "127.0.0.1:65432", 100*time.Millisecond, 200*time.Millisecond, nil)
	err := svc.Sync()
	if err == nil {
		t.Fatal("expected error querying non-existent NTP server, got nil")
	}

	st := svc.Status()
	if st.Synced {
		t.Errorf("expected Synced=false for unreachable server")
	}
	if st.Error == "" {
		t.Errorf("expected non-empty Error string in status")
	}
}

func TestServiceStartStop(t *testing.T) {
	svc := New(nil, "127.0.0.1:65432", 50*time.Millisecond, 50*time.Millisecond, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.Start(ctx)
	time.Sleep(120 * time.Millisecond)
	svc.Stop()

	st := svc.Status()
	if st.Synced {
		t.Errorf("expected Synced=false")
	}
}
