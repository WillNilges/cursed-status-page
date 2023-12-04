package main

func stringInSlice(searchSlice []string, searchString string) bool {
	for _, s := range searchSlice {
		if s == searchString {
			return true
		}
	}
	return false
}

// Compares a string you give it to a string passed in the config
func isRelevantReaction(reaction string) bool {
	switch reaction {
	case config.StatusOKEmoji, config.StatusWarnEmoji, config.StatusErrorEmoji:
		return true
	}
	return false
}
