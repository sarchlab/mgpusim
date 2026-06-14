package timing

import "github.com/sarchlab/akita/v5/hooking"

// HookPosBeforeEvent is a hook position that triggers before handling an event.
var HookPosBeforeEvent = &hooking.HookPos{Name: "BeforeEvent"}

// HookPosAfterEvent is a hook position that triggers after handling an event.
var HookPosAfterEvent = &hooking.HookPos{Name: "AfterEvent"}
