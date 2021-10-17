package models

import (
	"fmt"
	"time"
)

type Lap struct {
	TagID     string `json:"tag_id" xml:"TagID"`
	ReadCount int64  `xml:"ReadCount"`

	DiscoveryTime         string `xml:"DiscoveryTime"`
	DiscoveryTimePrepared time.Time

	LastSeenTime         string `xml:"LastSeenTime"`
	LastSeenTimePrepared time.Time

	Antenna   uint8  `json:"antenna" xml:"Antenna"`
	AntennaIP string `json:"antenna_ip"`
}

func (l Lap) String() string {
	return fmt.Sprintf("rfidID: %v, dsTime: %v, lsTime: %v, antenna %v, ip: %v",
		l.TagID,
		l.DiscoveryTimePrepared,
		l.LastSeenTimePrepared,
		l.Antenna,
		l.AntennaIP,
	)
}

type AverageLap struct {
	TagID string

	BestDiscoveryTime    time.Time
	AverageDiscoveryTime time.Time

	BestLastSeenTime    time.Time
	AverageLastSeenTime time.Time
}

func (l AverageLap) String() string {
	return fmt.Sprintf("tag: %v, bdt: %v, adt: %v, blst %v, alst: %v",
		l.TagID,
		l.BestDiscoveryTime,
		l.AverageDiscoveryTime,
		l.BestLastSeenTime,
		l.AverageLastSeenTime,
	)
}
