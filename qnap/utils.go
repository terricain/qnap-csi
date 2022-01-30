package qnap

func b2is(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func b2yn(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}