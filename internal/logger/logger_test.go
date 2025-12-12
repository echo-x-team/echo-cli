package logger

import (
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestPlainFormatter_TypePrefixAndFieldSkipping(t *testing.T) {
	ts := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	cases := []struct {
		name    string
		data    logrus.Fields
		message string
		want    string
	}{
		{
			name: "with type",
			data: logrus.Fields{
				"component":  "eq",
				"type":       "task.started",
				"caller":     "x.go:1",
				"payload":    "user_input",
				"session_id": "s1",
			},
			message: "published event into EQ",
			want:    "x.go:1 [2025-01-02T03:04:05Z] [INFO] [eq] [type=task.started] published event into EQ payload=user_input session_id=s1\n",
		},
		{
			name: "without type",
			data: logrus.Fields{
				"component": "eq",
				"caller":    "x.go:1",
				"foo":       "bar",
			},
			message: "hello",
			want:    "x.go:1 [2025-01-02T03:04:05Z] [INFO] [eq] hello foo=bar\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry := &logrus.Entry{
				Logger:  logrus.New(),
				Time:    ts,
				Level:   logrus.InfoLevel,
				Message: tc.message,
				Data:    tc.data,
			}
			out, err := (PlainFormatter{}).Format(entry)
			if err != nil {
				t.Fatalf("Format() error: %v", err)
			}
			got := string(out)
			if got != tc.want {
				t.Fatalf("unexpected format:\nwant: %q\ngot:  %q", tc.want, got)
			}
			if _, ok := tc.data["type"]; ok {
				if strings.Count(got, "type=task.started") != 1 {
					t.Fatalf("expected type to appear only once in output, got: %q", got)
				}
			}
		})
	}
}
