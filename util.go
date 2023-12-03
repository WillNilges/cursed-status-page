package main

func stringInSlice(searchSlice []string, searchString string) bool {
	for _, s := range searchSlice {
		if s == searchString {
			return true
		}
	}
	return false
}

