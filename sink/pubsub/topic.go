package pubsub

import (
	"fmt"
	"regexp"
	"strings"
)

// fullTopicPath is projects/<project>/topics/<topic>. Both ids are a single
// path segment: a slash belongs to the path, not to the id.
var fullTopicPath = regexp.MustCompile(`^projects/([^/]+)/topics/([^/]+)$`)

// ParseTopic resolves the Google Cloud project and topic id to publish to.
// topic is either a topic id, which needs project, or a full
// projects/<project>/topics/<topic> path. A project flag that names a
// different project than the path is an error.
func ParseTopic(project, topic string) (projectID, topicID string, err error) {
	project = strings.TrimSpace(project)
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "", "", fmt.Errorf("topic is required")
	}

	if m := fullTopicPath.FindStringSubmatch(topic); m != nil {
		if project != "" && project != m[1] {
			return "", "", fmt.Errorf("topic %q is in project %q, not %q", topic, m[1], project)
		}
		return m[1], m[2], nil
	}
	if strings.Contains(topic, "/") {
		return "", "", fmt.Errorf("topic %q must be a topic id or projects/<project>/topics/<topic>", topic)
	}
	if project == "" {
		return "", "", fmt.Errorf("project is required when the topic is not a projects/<project>/topics/<topic> path")
	}
	return project, topic, nil
}
