package caches

import "gitlab.com/akita/akita"

type subComponent interface {
	tick(now akita.VTimeInSec) bool
}
