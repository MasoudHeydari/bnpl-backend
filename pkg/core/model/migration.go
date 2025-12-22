package model

import "time"

type Migration struct {
	ID        uint64
	Name      string
	CreatedAt time.Time
}
