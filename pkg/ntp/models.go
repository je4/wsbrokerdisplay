package ntp

import "time"

// NTPStatus encapsulates the network time synchronization state and metrics.
type NTPStatus struct {
	Server        string        `json:"server"`
	Synced        bool          `json:"synced"`
	LastSync      time.Time     `json:"last_sync"`
	Time          time.Time     `json:"time"`
	Offset        time.Duration `json:"offset_nanos"`
	OffsetString  string        `json:"offset"`
	RTT           time.Duration `json:"rtt_nanos"`
	RTTString     string        `json:"rtt"`
	Stratum       int           `json:"stratum"`
	ReferenceID   string        `json:"reference_id"`
	Precision     string        `json:"precision"`
	LeapIndicator string        `json:"leap_indicator"`
	Error         string        `json:"error,omitempty"`
}
