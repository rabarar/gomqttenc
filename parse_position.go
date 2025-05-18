package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type PositionMessage struct {
	Envelope       MessageEnvelope
	LatitudeI      int
	LongitudeI     int
	Altitude       int
	Time           int64
	LocationSource string
	Timestamp      *int64 // optional
	SeqNumber      *int   // optional
	SatsInView     *int   // optional
	GroundSpeed    int
	GroundTrack    int
	PrecisionBits  int
}

func parsePositionMessage(msg string) (*PositionMessage, error) {
	//baseRe := regexp.MustCompile(`latitude_i:(-?\d+)\s+longitude_i:(-?\d+)\s+altitude:(\d+)\s+(?:time:(\d+)\s+)?\s+location_source:(\S+)(?:\s+\S+:\S+)*?\s*ground_speed:(\d+)\s+ground_track:(\d+)\s+(?:\S+:\S+\s+)*?precision_bits:(\d+)`)
	baseRe := regexp.MustCompile(`latitude_i:(-?\d+)\s+longitude_i:(-?\d+)\s+altitude:(\d+)\s+(?:time:(\d+)\s+)?location_source:(\S+)(?:\s+\S+:\S+)*?\s+ground_speed:(\d+)\s+ground_track:(\d+)(?:\s+\S+:\S+)*?\s+precision_bits:(\d+)`)
	baseMatch := baseRe.FindStringSubmatch(msg)
	if len(baseMatch) != 9 {
		return nil, fmt.Errorf("failed to match base POSITION fields")
	}

	atoi := func(s string) int {
		i, _ := strconv.Atoi(s)
		return i
	}

	atoi64 := func(s string) int64 {
		i, _ := strconv.ParseInt(s, 10, 64)
		return i
	}

	pos := &PositionMessage{
		LatitudeI:      atoi(baseMatch[1]),
		LongitudeI:     atoi(baseMatch[2]),
		Altitude:       atoi(baseMatch[3]),
		Time:           atoi64(baseMatch[4]),
		LocationSource: baseMatch[5],
		GroundSpeed:    atoi(baseMatch[6]),
		GroundTrack:    atoi(baseMatch[7]),
		PrecisionBits:  atoi(baseMatch[8]),
	}

	// Optional fields
	if strings.Contains(msg, "sats_in_view:") {
		re := regexp.MustCompile(`sats_in_view:(\d+)`)
		match := re.FindStringSubmatch(msg)
		if len(match) == 2 {
			sats := atoi(match[1])
			pos.SatsInView = &sats
		}
	}

	if strings.Contains(msg, "timestamp:") {
		re := regexp.MustCompile(`timestamp:(\d+)`)
		match := re.FindStringSubmatch(msg)
		if len(match) == 2 {
			ts := atoi64(match[1])
			pos.Timestamp = &ts
		}
	}

	if strings.Contains(msg, "seq_number:") {
		re := regexp.MustCompile(`seq_number:(\d+)`)
		match := re.FindStringSubmatch(msg)
		if len(match) == 2 {
			seq := atoi(match[1])
			pos.SeqNumber = &seq
		}
	}

	return pos, nil
}
