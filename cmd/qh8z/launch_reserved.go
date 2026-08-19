package main

func init() {
	for _, slug := range []string{
		"assets", "internal", "metrics",
		"terms", "privacy", "acceptable-use", "report-abuse", "security",
	} {
		reserved[slug] = true
	}
}
