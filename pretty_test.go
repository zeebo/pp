package pp

import (
	"testing"
	"time"
)

func TestPretty(t *testing.T) {
	t.Log(Sprint(struct {
		A [8]byte
		B *[8]byte
		C struct {
			data [8]byte
		}
		D *time.Time
		E []int
		F []int
		G map[string]bool
		H map[string]byte
		I interface{}
		J interface{}
	}{
		B: new([8]byte),
		D: new(time.Time),
		F: make([]int, 10),
		H: make(map[string]byte),
		J: 10,
	}))
}
