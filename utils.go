package main

import "strings"

func getLastTopicElement(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func getLastNTopicSegments(topic string, n int) string {
	parts := strings.Split(topic, "/")
	if len(parts) == 0 {
		return ""
	}

	// Clamp n to available parts
	start := len(parts) - (n + 1)
	if start < 0 {
		start = 0
	}

	return strings.Join(parts[start:], "/")
}

func getNthTopicSegmentFromEnd(topic string, n int) string {
	parts := strings.Split(topic, "/")
	if len(parts) == 0 {
		return ""
	}

	index := len(parts) - 1 - n
	if index < 0 || index >= len(parts) {
		return ""
	}

	return parts[index]
}
