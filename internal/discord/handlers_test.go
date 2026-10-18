package discord

import "testing"

func TestAutoSpeaker(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{name: "question", text: "これは本当ですか？", want: 8},
		{name: "greeting", text: "おはようございます", want: 1},
		{name: "thanks", text: "ありがとう", want: 3},
		{name: "default", text: "読み上げテストです", want: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := autoSpeaker(test.text); got != test.want {
				t.Fatalf("autoSpeaker(%q) = %d, want %d", test.text, got, test.want)
			}
		})
	}
}

func TestPCMFrames(t *testing.T) {
	data := make([]byte, 4000)
	frames := pcmFrames(data)
	if len(frames) != 2 {
		t.Fatalf("pcmFrames returned %d frames, want 2", len(frames))
	}
	if len(frames[0]) != 3840 || len(frames[1]) != 3840 {
		t.Fatalf("unexpected frame sizes: %d, %d", len(frames[0]), len(frames[1]))
	}
}
