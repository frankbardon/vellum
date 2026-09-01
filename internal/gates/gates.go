// Package gates holds the source-level firewalls.
//
// These are tests that assert things about the *source* rather than about
// behaviour: that nothing imports a package which would break determinism, that
// no output path reads the clock, that no ordered emission ranges a map. They
// live together because they share a walker and because a reader looking for
// "what is structurally forbidden here" should find one answer rather than
// several scattered through the packages they constrain.
//
// Every one of them exists because the thing it forbids is invisible in review
// and catastrophic in output. A single time.Now on a write path does not fail a
// test — it produces a document that is correct, that opens, and that differs
// from the one produced a second earlier.
package gates
