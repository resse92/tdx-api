package tdx

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"
)

var ErrInvalidBarRange = errors.New("无效的 K 线日期范围")

var shanghaiLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

const calendarSafetyDays = 14

// PlanBars converts an inclusive calendar range to the upstream's newest-first page.
// Calendar days intentionally over-fetch weekends and holidays; the caller filters bars.
func PlanBars(start, end uint64, period string, now time.Time) (uint32, uint32, error) {
	startDate, ok := ymd(datePart(start))
	if !ok {
		return 0, 0, ErrInvalidBarRange
	}
	endDate, ok := ymd(datePart(end))
	if !ok || startDate.After(endDate) {
		return 0, 0, ErrInvalidBarRange
	}
	today := dateOnly(now)
	offsetDays := 0
	if endDate.Before(today) {
		offsetDays = int(today.Sub(endDate)/(24*time.Hour)) - calendarSafetyDays
		if offsetDays < 0 {
			offsetDays = 0
		}
	}
	spanDays := int(endDate.Sub(startDate)/(24*time.Hour)) + 1
	if spanDays > 3650 {
		return 0, 0, ErrInvalidBarRange
	}
	factor := barsPerDay(period)
	limit := uint64(spanDays+calendarSafetyDays) * uint64(factor)
	if limit == 0 || limit > math.MaxUint16 || offsetDays > math.MaxUint32 {
		return 0, 0, ErrInvalidBarRange
	}
	return uint32(offsetDays), uint32(limit), nil
}

func datePart(value uint64) uint32 {
	if value <= 99999999 {
		return uint32(value)
	}
	return uint32(value / 1000000)
}

func ymd(value uint32) (time.Time, bool) {
	y, m, d := int(value/10000), time.Month(value/100%100), int(value%100)
	if y < 1990 || y > 2100 || m < time.January || m > time.December || d < 1 {
		return time.Time{}, false
	}
	date := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return date, date.Year() == y && date.Month() == m && date.Day() == d
}

func dateOnly(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func barsPerDay(period string) uint32 {
	switch period {
	case "1m":
		return 240
	case "5m":
		return 48
	case "15m":
		return 16
	case "30m":
		return 8
	case "1h":
		return 4
	default:
		return 1
	}
}

// FilterBars retains the original response shape and order while removing bars
// outside the requested inclusive date range.
func FilterBars(data any, start, end uint64) any {
	startDate, startOK := boundaryTime(start, false)
	endDate, endOK := boundaryTime(end, true)
	if !startOK || !endOK {
		return data
	}
	value := filterValue(reflect.ValueOf(data), startDate, endDate)
	if !value.IsValid() {
		return data
	}
	return value.Interface()
}

func boundaryTime(value uint64, end bool) (time.Time, bool) {
	if value <= 99999999 {
		date, ok := ymd(uint32(value))
		if !ok {
			return time.Time{}, false
		}
		if end {
			return date.Add(24*time.Hour - time.Nanosecond), true
		}
		return date, true
	}
	if value > 99999999999999 {
		return time.Time{}, false
	}
	text := fmt.Sprintf("%014d", value)
	date, err := time.ParseInLocation("20060102150405", text, shanghaiLocation)
	return date, err == nil
}

func filterValue(value reflect.Value, start, end time.Time) reflect.Value {
	if !value.IsValid() {
		return value
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return value
		}
		out := reflect.New(value.Elem().Type())
		out.Elem().Set(value.Elem())
		if list := out.Elem().FieldByName("List"); list.IsValid() && list.CanSet() && list.Kind() == reflect.Slice {
			list.Set(filterSlice(list, start, end))
		}
		return out
	}
	if value.Kind() == reflect.Slice {
		return filterSlice(value, start, end)
	}
	return value
}

func filterSlice(value reflect.Value, start, end time.Time) reflect.Value {
	out := reflect.MakeSlice(value.Type(), 0, value.Len())
	for i := 0; i < value.Len(); i++ {
		item := value.Index(i)
		candidate := item
		if candidate.Kind() == reflect.Pointer {
			if candidate.IsNil() {
				continue
			}
			candidate = candidate.Elem()
		}
		date, ok := barDate(candidate)
		if !ok || date.Before(start) || date.After(end) {
			continue
		}
		out = reflect.Append(out, item)
	}
	return out
}

func barDate(value reflect.Value) (time.Time, bool) {
	if value.Kind() != reflect.Struct {
		return time.Time{}, false
	}
	if field := value.FieldByName("DateTime"); field.IsValid() && field.Type() == reflect.TypeOf(time.Time{}) {
		date := field.Interface().(time.Time)
		if !date.IsZero() {
			return date, true
		}
	}
	year, month, day := value.FieldByName("Year"), value.FieldByName("Month"), value.FieldByName("Day")
	if year.IsValid() && month.IsValid() && day.IsValid() && year.Kind() == reflect.Int && month.Kind() == reflect.Int && day.Kind() == reflect.Int {
		return time.Date(int(year.Int()), time.Month(month.Int()), int(day.Int()), 0, 0, 0, 0, time.UTC), true
	}
	return time.Time{}, false
}
