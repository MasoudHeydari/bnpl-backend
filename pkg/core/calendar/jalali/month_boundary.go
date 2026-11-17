package jalali

import (
	"fmt"
	"time"
)

// StartOfFollowingTehranMonth returns the inclusive beginning of the next
// Jalali month in Asia/Tehran. It is a domain calendar boundary, NOT a fixed
// 30-day duration, and is independent of the server's timezone.
func StartOfFollowingTehranMonth(at time.Time) (time.Time,error){
	if at.IsZero(){return time.Time{},fmt.Errorf("monthly credit cutoff: anchor is required")}
	loc,err:=time.LoadLocation("Asia/Tehran")
	if err!=nil{return time.Time{},fmt.Errorf("monthly credit cutoff: timezone: %w",err)}
	date:=FromTime(at.In(loc))
	date.Month++
	if date.Month>12 {
		date.Year++
		date.Month=1
	}
	date.Day=1
	return date.ToTime(time.Date(2000,time.January,1,0,0,0,0,loc),loc),nil
}
