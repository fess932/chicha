package rfid

import (
	"bytes"
	"chicha/Models"
	"chicha/Packages/rfid/models"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parse conf
// readonly
var (
	tz            *time.Location
	xmlTimeFormat = "2006/01/02 15:04:05.000"
)

func decode(data []byte) (models.Lap, error) {
	if Models.IsValidXML(data) {
		return decodeXML(data)
	}

	return decodeCSV(data)
}

func decodeXML(data []byte) (lap models.Lap, err error) {
	err = xml.Unmarshal(data, &lap)
	if err != nil {
		return
	}

	discoveryTime, err := time.ParseInLocation(xmlTimeFormat, lap.DiscoveryTime, tz)
	if err != nil {
		return models.Lap{}, fmt.Errorf("time.ParseInLocation dsTime: %w", err)
	}

	lastSeenTime, err := time.ParseInLocation(xmlTimeFormat, lap.LastSeenTime, tz)
	if err != nil {
		return models.Lap{}, fmt.Errorf("time.ParseInLocation lsTime: %w", err)
	}

	lap.TagID = strings.ReplaceAll(lap.TagID, " ", "")
	lap.DiscoveryTimePrepared = discoveryTime
	lap.LastSeenTimePrepared = lastSeenTime
	// time

	return
}

func decodeCSV(data []byte) (lap models.Lap, err error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = ','
	r.FieldsPerRecord = 3
	CSV, err := r.Read()
	if err != nil {
		return models.Lap{}, err
	}

	t, err := decodeTime(CSV[1])
	if err != nil {
		return models.Lap{}, err
	}

	// Prepare antenna position
	antennaPosition, err := strconv.ParseUint(strings.TrimSpace(CSV[2]), 10, 8)
	if err != nil {
		return models.Lap{}, fmt.Errorf("recived incorrect Antenna position CSV value: %w", err)
	}

	lap.TagID = CSV[0]
	lap.DiscoveryTimePrepared = t
	lap.Antenna = uint8(antennaPosition)

	return
}

func decodeTime(ms string) (time.Time, error) {
	msInt, err := strconv.ParseInt(strings.TrimSpace(ms), 10, 64)
	if err != nil {
		return time.Time{}, err
	}

	return time.Unix(0, msInt*int64(time.Millisecond)), nil
}
