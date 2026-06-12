// Package addresstranslator implements a component that translates the
// addresses of read and write requests from virtual to physical and forwards
// the translated requests to the bottom memory unit.
//
// It is a variant of akita's mem/vm/addresstranslator that additionally
// coalesces translation requests targeting the same virtual page.
package addresstranslator
