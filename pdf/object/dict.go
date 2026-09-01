package object

// Dict is a PDF dictionary held as an ordered sequence of entries.
//
// A slice rather than a map, deliberately. A map would have to be sorted before
// writing, and a sort is a rule about output that lives somewhere other than
// where the output is built — the kind of rule that is correct until somebody
// adds a second write path. Holding the order in the value makes key order a
// property of the code that built the dictionary rather than of the run, which
// is the difference between nondeterminism being unrepresentable and being
// tested against.
//
// It also keeps dictionaries readable. /Type and /Subtype come first because
// they were set first, which is where a person looking at a hex dump expects
// them, and where the specification's own examples put them.
//
// The zero value is an empty dictionary ready for use.
type Dict struct {
	entries []entry
}

type entry struct {
	key   Name
	value Object
}

// NewDict returns a dictionary with the given entries, in the order supplied.
//
// Entries are pairs: NewDict("Type", Name("Page"), "Parent", parent). An odd
// count, or a key that is not a Name, panics — both are programming errors in
// code that is written once and read many times, and returning an error would
// put an err check on every dictionary literal in the package.
func NewDict(pairs ...any) Dict {
	if len(pairs)%2 != 0 {
		panic("object: NewDict needs an even number of arguments, as key/value pairs")
	}
	d := Dict{entries: make([]entry, 0, len(pairs)/2)}
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(Name)
		if !ok {
			if s, isString := pairs[i].(string); isString {
				key = Name(s)
			} else {
				panic("object: NewDict key must be a Name or a string")
			}
		}
		value, ok := pairs[i+1].(Object)
		if !ok {
			panic("object: NewDict value for /" + string(key) + " must be an Object")
		}
		d.Set(key, value)
	}
	return d
}

// Set adds or replaces an entry.
//
// Replacing keeps the original position rather than moving the key to the end,
// so a dictionary built by one path and then adjusted by another produces the
// same bytes as one built with the final value in the first place.
func (d *Dict) Set(key Name, value Object) {
	for i := range d.entries {
		if d.entries[i].key == key {
			d.entries[i].value = value
			return
		}
	}
	d.entries = append(d.entries, entry{key: key, value: value})
}

// SetIf adds the entry only when cond holds.
//
// The alternative at the call site is an if statement per optional key, which
// buries the shape of the dictionary in control flow.
func (d *Dict) SetIf(cond bool, key Name, value Object) {
	if cond {
		d.Set(key, value)
	}
}

// Get returns the value stored under key. The second result reports presence.
func (d Dict) Get(key Name) (Object, bool) {
	for _, e := range d.entries {
		if e.key == key {
			return e.value, true
		}
	}
	return nil, false
}

// Len returns the number of entries.
func (d Dict) Len() int { return len(d.entries) }

// Keys returns the keys in write order.
func (d Dict) Keys() []Name {
	out := make([]Name, len(d.entries))
	for i, e := range d.entries {
		out[i] = e.key
	}
	return out
}

// AppendPDF implements [Object].
func (d Dict) AppendPDF(dst []byte) []byte {
	dst = append(dst, "<<"...)
	for _, e := range d.entries {
		dst = e.key.AppendPDF(dst)
		// A space between key and value is needed only when the value would
		// otherwise run into the key. It is written unconditionally because the
		// bytes are cheap and the alternative is a rule about value syntax
		// living in the dictionary writer.
		dst = append(dst, ' ')
		dst = e.value.AppendPDF(dst)
	}
	return append(dst, ">>"...)
}
