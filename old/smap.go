package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type SmapProperties struct {
	UnitOfTime    UnitOfTime
	UnitOfMeasure string
	StreamType    StreamType
}

func (sp SmapProperties) MarshalJSON() ([]byte, error) {
	// watch capitals
	var (
		m     map[string]string
		empty bool = true
	)
	if sp.UnitOfTime != 0 {
		empty = false
		if len(m) == 0 {
			m = make(map[string]string)
		}
		m["UnitofTime"] = sp.UnitOfTime.String()
	}
	if sp.StreamType != 0 {
		empty = false
		if len(m) == 0 {
			m = make(map[string]string)
		}
		m["StreamType"] = sp.StreamType.String()
	}
	if len(sp.UnitOfMeasure) != 0 {
		empty = false
		if len(m) == 0 {
			m = make(map[string]string)
		}
		m["UnitofMeasure"] = sp.UnitOfMeasure
	}
	if !empty {
		return json.Marshal(m)
	} else {
		return json.Marshal(nil)
	}
}

func (sp SmapProperties) IsEmpty() bool {
	return sp.UnitOfTime == 0 &&
		sp.UnitOfMeasure == "" &&
		sp.StreamType == 0
}

type SmapMessage struct {
	Path       string          `json:",omitempty" msgpack:",omitempty"`
	UUID       UUID            `json:"uuid,omitempty" msgpack:",omitempty"`
	Properties *SmapProperties `json:",omitempty" msgpack:",omitempty"`
	Actuator   Dict            `json:",omitempty" msgpack:",omitempty"`
	Metadata   Dict            `json:",omitempty" msgpack:",omitempty"`
	Readings   []Reading       `json:",omitempty" msgpack:",omitempty"`
}

// will insert a key string e.g. "Metadata.KeyName" and value e.g. "Value"
// into a SmapMessage instance, creating one if necessary
func (msg *SmapMessage) AddTag(key string, value string) {
	if msg == nil {
		msg = &SmapMessage{}
	}
	switch {
	case strings.HasPrefix(key, "Metadata."):
		if len(msg.Metadata) == 0 {
			msg.Metadata = Dict{}
		}
		// len("Metadata.") == 9
		msg.Metadata[key[9:]] = value
	case strings.HasPrefix(key, "Actuator."):
		if len(msg.Actuator) == 0 {
			msg.Actuator = Dict{}
		}
		// len("Actuator.") == 9
		msg.Actuator[key[9:]] = value
	case key == "Path":
		msg.Path = value
	case key == "UUID":
		msg.UUID = UUID(value)
		//case strings.HasPrefix(key, "Properties."):
		//    if msg.Properties == nil {
		//        msg.Properties = &SmapProperties{}
		//    }
		//    switch key {
		//    case "Properties.UnitofTime":
		//        msg.Properties.UnitOfTime = value
		//    }
		//    msg.Actuator[key[11:]] = value
	}
}

func (sm *SmapMessage) UnmarshalJSON(b []byte) (err error) {
	var (
		incoming   = new(incomingSmapMessage)
		time       uint64
		time_weird float64
		value_num  float64
		value_obj  interface{}
	)

	// unmarshal to an intermediary struct that matches the format
	// of the incoming messages
	err = json.Unmarshal(b, incoming)
	if err != nil {
		return
	}

	// copy the values over that we don't need to translate
	sm.UUID = incoming.UUID
	sm.Path = incoming.Path
	if len(incoming.Metadata) > 0 {
		sm.Metadata = flatten(incoming.Metadata)
	}
	if !incoming.Properties.IsEmpty() {
		sm.Properties = &incoming.Properties
	}
	if len(incoming.Actuator) > 0 {
		sm.Actuator = flatten(incoming.Actuator)
	}

	// convert the readings depending if they are numeric or object
	sm.Readings = make([]Reading, len(incoming.Readings))
	idx := 0
	for _, reading := range incoming.Readings {
		if len(reading) == 0 {
			continue
		}
		// time should be a uint64 no matter what
		err = json.Unmarshal(reading[0], &time)
		if err != nil {
			err = json.Unmarshal(reading[0], &time_weird)
			if err != nil {
				return
			} else {
				time = uint64(time_weird)
			}
		}

		// check if we have a numerical value
		err = json.Unmarshal(reading[1], &value_num)

		// the unit of time for these readings
		var uot UnitOfTime
		if sm.Properties == nil || sm.Properties.IsEmpty() {
			// if we don't have info, then calculate from time
			uot = GuessTimeUnit(time)
		} else {
			uot = sm.Properties.UnitOfTime
		}

		if err != nil {
			// if we don't, then we treat as an object reading
			err = json.Unmarshal(reading[1], &value_obj)
			sm.Readings[idx] = &SmapObjectReading{time, uot, value_obj}
		} else {
			sm.Readings[idx] = &SmapNumberReading{time, uot, value_num}
		}
		idx += 1
	}
	sm.Readings = sm.Readings[:idx]
	return
}

// returns True if the message contains anything beyond Path, UUID, Readings
func (msg *SmapMessage) HasMetadata() bool {
	return (msg.Actuator != nil && len(msg.Actuator) > 0) ||
		(msg.Metadata != nil && len(msg.Metadata) > 0) ||
		(msg.Properties != nil && !msg.Properties.IsEmpty())
}

func (msg *SmapMessage) IsTimeseries() bool {
	return msg.UUID != ""
}

func (msg SmapMessage) IsResult() {}

type SmapMessageList []*SmapMessage

func (sml SmapMessageList) IsResult() {}

type TieredSmapMessage map[string]*SmapMessage

// This performs the metadata inheritance for the paths and messages inside
// this collection of SmapMessages. Inheritance starts from the root path "/"
// can progresses towards the leaves.
// First, get a list of all of the potential timeseries (any path that contains a UUID)
// Then, for each of the prefixes for the path of that timeserie (util.getPrefixes), grab
// the paths from the TieredSmapMessage that match the prefixes. Sort these in "decreasing" order
// and apply to the metadata.
// Finally, delete all non-timeseries paths
func (tsm *TieredSmapMessage) CollapseToTimeseries() {
	var (
		prefixMsg *SmapMessage
		found     bool
	)
	for path, msg := range *tsm {
		if !msg.IsTimeseries() {
			continue
		}
		prefixes := getPrefixes(path)
		sort.Sort(sort.Reverse(sort.StringSlice(prefixes)))
		for _, prefix := range prefixes {
			// if we don't find the prefix OR it exists but doesn't have metadata, we skip
			prefixMsg, found = (*tsm)[prefix]
			if !found || prefixMsg == nil || (prefixMsg != nil && !prefixMsg.HasMetadata()) {
				continue
			}
			// otherwise, we apply keys from paths higher up if our timeseries doesn't already have the key
			// (this is reverse inheritance)
			if prefixMsg.Metadata != nil && len(prefixMsg.Metadata) > 0 {
				for k, v := range prefixMsg.Metadata {
					if _, hasKey := msg.Metadata[k]; !hasKey {
						if msg.Metadata == nil {
							msg.Metadata = make(Dict)
						}
						msg.Metadata[k] = v
					}
				}
			}
			if prefixMsg.Properties != nil && !prefixMsg.Properties.IsEmpty() {
				if msg.Properties.UnitOfTime != 0 {
					msg.Properties.UnitOfTime = prefixMsg.Properties.UnitOfTime
				}
				if msg.Properties.UnitOfMeasure != "" {
					msg.Properties.UnitOfMeasure = prefixMsg.Properties.UnitOfMeasure
				}
				if msg.Properties.StreamType != 0 {
					msg.Properties.StreamType = prefixMsg.Properties.StreamType
				}
			}

			if prefixMsg.Actuator != nil && len(prefixMsg.Actuator) > 0 {
				for k, v := range prefixMsg.Actuator {
					if _, hasKey := msg.Actuator[k]; !hasKey {
						if msg.Actuator == nil {
							msg.Actuator = make(Dict)
						}
						msg.Actuator[k] = v
					}
				}
			}
			(*tsm)[path] = msg
		}
	}
	// when done, delete all non timeseries paths
	for path, msg := range *tsm {
		if !msg.IsTimeseries() {
			delete(*tsm, path)
		}
	}
}

type incomingSmapMessage struct {
	// Readings for this message
	Readings [][]json.RawMessage
	// If this struct corresponds to a sMAP collection,
	// then Contents contains a list of paths contained within
	// this collection
	Contents []string `json:",omitempty"`
	// Map of the metadata
	Metadata Dict `json:",omitempty"`
	// Map containing the actuator reference
	Actuator Dict `json:",omitempty"`
	// Map of the properties
	Properties SmapProperties `json:",omitempty"`
	// Unique identifier for this stream. Should be empty for Collections
	UUID UUID `json:"uuid"`
	// Path of this stream (thus far)
	Path string `json:"Path"`
}

// internal unique identifier
type UUID string

// stream type indicators
type StreamType uint

const (
	OBJECT_STREAM StreamType = iota + 1
	NUMERIC_STREAM
)

func (st StreamType) String() string {
	switch st {
	case OBJECT_STREAM:
		return "object"
	case NUMERIC_STREAM:
		return "numeric"
	default:
		return ""
	}
}

func (st StreamType) MarshalJSON() ([]byte, error) {
	switch st {
	case OBJECT_STREAM:
		return []byte(`"object"`), nil
	case NUMERIC_STREAM:
		return []byte(`"numeric"`), nil
	default:
		return []byte(`"numeric"`), nil
	}
}

func (st *StreamType) UnmarshalJSON(b []byte) (err error) {
	str := strings.Trim(string(b), `"`)
	switch str {
	case "numeric":
		*st = NUMERIC_STREAM
	case "object":
		*st = OBJECT_STREAM
	default:
		return fmt.Errorf("%v is not a valid StreamType", str)
	}
	return nil
}

var TimeConvertErr = errors.New("Over/underflow error in converting time")

func ParseUOT(units string) (UnitOfTime, error) {
	switch units {
	case "s", "sec", "second", "seconds":
		return UOT_S, nil
	case "us", "usec", "microsecond", "microseconds":
		return UOT_US, nil
	case "ms", "msec", "millisecond", "milliseconds":
		return UOT_MS, nil
	case "ns", "nsec", "nanosecond", "nanoseconds":
		return UOT_NS, nil
	default:
		return UOT_S, fmt.Errorf("Invalid unit %v. Must be s,us,ms,ns", units)
	}
}

// unit of time indicators
type UnitOfTime uint

const (
	// nanoseconds 1000000000
	UOT_NS UnitOfTime = 1
	// microseconds 1000000
	UOT_US UnitOfTime = 2
	// milliseconds 1000
	UOT_MS UnitOfTime = 3
	// seconds 1
	UOT_S UnitOfTime = 4
)

var unitmultiplier = map[UnitOfTime]uint64{
	UOT_NS: 1000000000,
	UOT_US: 1000000,
	UOT_MS: 1000,
	UOT_S:  1,
}

// Takes a timestamp with accompanying unit of time 'stream_uot' and
// converts it to the unit of time 'target_uot'
func ConvertTime(time uint64, stream_uot, target_uot UnitOfTime) (uint64, error) {
	var returnTime uint64
	if stream_uot == target_uot {
		return time, nil
	}
	if target_uot < stream_uot { // target/stream is > 1, so we can use uint64
		returnTime = time * (unitmultiplier[target_uot] / unitmultiplier[stream_uot])
		if returnTime < time {
			return time, TimeConvertErr
		}
	} else {
		returnTime = time / uint64(unitmultiplier[stream_uot]/unitmultiplier[target_uot])
		if returnTime > time {
			return time, TimeConvertErr
		}
	}
	return returnTime, nil
}

func (u UnitOfTime) String() string {
	switch u {
	case UOT_NS:
		return "ns"
	case UOT_US:
		return "us"
	case UOT_MS:
		return "ms"
	case UOT_S:
		return "s"
	default:
		return ""
	}
}

func (u UnitOfTime) MarshalJSON() ([]byte, error) {
	switch u {
	case UOT_NS:
		return []byte(`"ns"`), nil
	case UOT_US:
		return []byte(`"us"`), nil
	case UOT_MS:
		return []byte(`"ms"`), nil
	case UOT_S:
		return []byte(`"s"`), nil
	default:
		return []byte(`"s"`), nil
	}
}

func (u *UnitOfTime) UnmarshalJSON(b []byte) (err error) {
	str := strings.Trim(string(b), `"`)
	switch str {
	case "ns":
		*u = UOT_NS
	case "us":
		*u = UOT_US
	case "ms":
		*u = UOT_MS
	case "s":
		*u = UOT_S
	default:
		return fmt.Errorf("%v is not a valid UnitOfTime", str)
	}
	return nil
}

func ParseAbsTime(num, units string) (time.Time, error) {
	var d time.Time
	var err error
	i, err := strconv.ParseUint(num, 10, 64)
	if err != nil {
		return d, err
	}
	uot, err := ParseUOT(units)
	if err != nil {
		return d, err
	}
	unixseconds, err := ConvertTime(i, uot, UOT_S)
	if err != nil {
		return d, err
	}
	tmp, err := ConvertTime(unixseconds, UOT_S, uot)
	if err != nil {
		return d, err
	}
	leftover := i - tmp
	unixns, err := ConvertTime(leftover, uot, UOT_NS)
	if err != nil {
		return d, err
	}
	d = time.Unix(int64(unixseconds), int64(unixns))
	return d, err
}

func ParseReltime(num, units string) (time.Duration, error) {
	var d time.Duration
	i, err := strconv.ParseInt(num, 10, 64)
	if err != nil {
		return d, err
	}
	d = time.Duration(i)
	switch units {
	case "h", "hr", "hour", "hours":
		d *= time.Hour
	case "m", "min", "minute", "minutes":
		d *= time.Minute
	case "s", "sec", "second", "seconds":
		d *= time.Second
	case "us", "usec", "microsecond", "microseconds":
		d *= time.Microsecond
	case "ms", "msec", "millisecond", "milliseconds":
		d *= time.Millisecond
	case "ns", "nsec", "nanosecond", "nanoseconds":
		d *= time.Nanosecond
	case "d", "day", "days":
		d *= 24 * time.Hour
	default:
		err = fmt.Errorf("Invalid unit %v. Must be h,m,s,us,ms,ns,d", units)
	}
	return d, err
}

// Takes 2 durations and returns the result of them added together
func AddDurations(d1, d2 time.Duration) time.Duration {
	d1nano := d1.Nanoseconds()
	d2nano := d2.Nanoseconds()
	res := d1nano + d2nano
	return time.Duration(res) * time.Nanosecond
}

// a flat map for storing key-value pairs
type Dict map[string]interface{}

func NewDict() *Dict {
	return new(Dict)
}

// interface for sMAP readings
type Reading interface {
	GetTime() uint64
	ConvertTime(to UnitOfTime) error
	SetUOT(uot UnitOfTime)
	GetValue() interface{}
	IsObject() bool
	IsStats() bool
}

// Reading implementation for numerical data
type SmapNumberReading struct {
	// uint64 timestamp
	Time uint64
	UoT  UnitOfTime
	// value associated with this timestamp
	Value float64
}

func (s *SmapNumberReading) MarshalJSON() ([]byte, error) {
	floatString := strconv.FormatFloat(s.Value, 'f', -1, 64)
	timeString := strconv.FormatUint(s.Time, 10)
	return json.Marshal([]json.Number{json.Number(timeString), json.Number(floatString)})
}

func (s *SmapNumberReading) GetTime() uint64 {
	return s.Time
}

func (s *SmapNumberReading) SetUOT(uot UnitOfTime) {
	s.UoT = uot
}

func (s *SmapNumberReading) ConvertTime(to_uot UnitOfTime) (err error) {
	guess := GuessTimeUnit(s.Time)
	if to_uot != guess {
		s.Time, err = convertTime(s.Time, guess, to_uot)
		s.UoT = guess
	}
	return
}

func (s *SmapNumberReading) IsObject() bool {
	return false
}

func (s *SmapNumberReading) IsStats() bool {
	return false
}

func (s *SmapNumberReading) GetValue() interface{} {
	return s.Value
}

// Reading implementation for object data
type SmapObjectReading struct {
	// uint64 timestamp
	Time uint64
	UoT  UnitOfTime
	// value associated with this timestamp
	Value interface{}
}

func (s *SmapObjectReading) MarshalJSON() ([]byte, error) {
	timeString := strconv.FormatUint(s.Time, 10)
	return json.Marshal([]interface{}{json.Number(timeString), s.Value})
}

func (s *SmapObjectReading) GetTime() uint64 {
	return s.Time
}

func (s *SmapObjectReading) ConvertTime(to_uot UnitOfTime) (err error) {
	guess := GuessTimeUnit(s.Time)
	if to_uot != guess {
		s.Time, err = convertTime(s.Time, guess, to_uot)
		s.UoT = guess
	}
	return
}

func (s *SmapObjectReading) IsObject() bool {
	return true
}

func (s *SmapObjectReading) IsStats() bool {
	return false
}

func (s *SmapObjectReading) GetValue() interface{} {
	return s.Value
}

func (s *SmapObjectReading) SetUOT(uot UnitOfTime) {
	s.UoT = uot
}

type StatisticalNumberReading struct {
	Time  uint64
	UoT   UnitOfTime
	Count uint64
	Min   float64
	Mean  float64
	Max   float64
}

func (s *StatisticalNumberReading) IsObject() bool {
	return false
}

func (s *StatisticalNumberReading) IsStats() bool {
	return true
}

func (s *StatisticalNumberReading) GetValue() interface{} {
	return map[string]interface{}{"Count": s.Count, "Min": s.Min, "Mean": s.Mean, "Max": s.Max}
}

func (s *StatisticalNumberReading) SetUOT(uot UnitOfTime) {
	s.UoT = uot
}

func (s *StatisticalNumberReading) ConvertTime(to_uot UnitOfTime) (err error) {
	guess := GuessTimeUnit(s.Time)
	if to_uot != guess {
		s.Time, err = convertTime(s.Time, guess, to_uot)
		s.UoT = guess
	}
	return
}

func (s *StatisticalNumberReading) MarshalJSON() ([]byte, error) {
	timeString := strconv.FormatUint(s.Time, 10)
	return json.Marshal([]interface{}{json.Number(timeString), s.Count, s.Min, s.Mean, s.Max})
}

func (s *StatisticalNumberReading) GetTime() uint64 {
	return s.Time
}

type SmapNumbersResponse struct {
	Readings []*SmapNumberReading
	UUID     UUID `json:"uuid"`
}

type SmapObjectResponse struct {
	Readings []*SmapObjectReading
	UUID     UUID `json:"uuid"`
}

type StatisticalNumbersResponse struct {
	Readings []*StatisticalNumberReading
	UUID     UUID `json:"uuid"`
}

// Takes a dictionary that contains nested dictionaries and
// transforms it to a 1-level map with fields separated by periods k.kk.kkk = v
func flatten(m Dict) Dict {
	var ret = make(Dict)
	for k, v := range m {
		if vb, ok := v.(map[string]interface{}); ok {
			for kk, vv := range flatten(vb) {
				ret[k+"."+kk] = vv
			}
		} else {
			ret[k] = v
		}
	}
	return ret
}

// get current time
func GetNow(units UnitOfTime) uint64 {
	now, err := convertTime(uint64(time.Now().UnixNano()), UOT_NS, units)
	if err != nil {
		panic(err) // "This should never ever happen" - Gabe Fierro, 8 February 2016
	}
	return now
}

const (
	S_LOW  uint64 = 2 << 30
	MS_LOW uint64 = 2 << 39
	US_LOW uint64 = 2 << 50
	NS_LOW uint64 = 2 << 58
)

func GuessTimeUnit(val uint64) UnitOfTime {
	if val < MS_LOW {
		return UOT_S
	} else if val < US_LOW {
		return UOT_MS
	} else if val < NS_LOW {
		return UOT_US
	}
	return UOT_NS
}

// Given a forward-slash delimited path, returns a slice of prefixes, e.g.:
// input: /a/b/c/d
// output: ['/', '/a','/a/b','/a/b/c']
func getPrefixes(s string) []string {
	ret := []string{"/"}
	root := ""
	s = "/" + s
	for _, prefix := range strings.Split(s, "/") {
		if len(prefix) > 0 { //skip empty strings created by Split
			root += "/" + prefix
			ret = append(ret, root)
		}
	}
	if len(ret) > 1 {
		return ret[:len(ret)-1]
	}
	return ret
}

// Takes a timestamp with accompanying unit of time 'stream_uot' and
// converts it to the unit of time 'target_uot'
func convertTime(time uint64, stream_uot, target_uot UnitOfTime) (uint64, error) {
	var returnTime uint64
	if stream_uot == target_uot {
		return time, nil
	}
	if target_uot < stream_uot { // target/stream is > 1, so we can use uint64
		returnTime = time * (unitmultiplier[target_uot] / unitmultiplier[stream_uot])
		if returnTime < time {
			return time, TimeConvertErr
		}
	} else {
		returnTime = time / uint64(unitmultiplier[stream_uot]/unitmultiplier[target_uot])
		if returnTime > time {
			return time, TimeConvertErr
		}
	}
	return returnTime, nil
}
