package connectedpolicy

// These are the v255 typed empty arrays, in the fixed query order. Credential
// content is neither retained nor included in errors. Reject at the first byte
// of drift, including a nonzero array count. No input is copied or retained.
const emptyCredentials = "a(ss) 0\na(ss) 0\na(say) 0\na(say) 0\nas 0\n"

type emptyCredentialsOutput struct {
	position int
	failed   bool
}

func (o *emptyCredentialsOutput) Write(p []byte) (int, error) {
	if o.failed {
		return 0, ErrUnsafe
	}
	for i, b := range p {
		if o.position >= len(emptyCredentials) || b != emptyCredentials[o.position] {
			o.failed = true
			return i, ErrUnsafe
		}
		o.position++
	}
	return len(p), nil
}

func (o *emptyCredentialsOutput) complete() bool {
	return !o.failed && o.position == len(emptyCredentials)
}
