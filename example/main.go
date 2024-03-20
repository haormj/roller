package main

import (
	"fmt"
	"log"
	"time"

	"github.com/haormj/roller/v2"
)

func main() {
	r, err := roller.NewRoller(
		roller.Filename("./test.log"),
		roller.Size(1024),
		roller.Duration(30*time.Second),
		roller.LifecycleGlob("./test/test_*.log"),
		roller.LifecycleCount(10),
		roller.LifecycleDuration(time.Minute),
		roller.LifecycleSize(10*1024),
		roller.RotateName(func(s string) string {
			return fmt.Sprintf("./test/test_%d.log", time.Now().UnixMilli())
		}),
	)
	if err != nil {
		log.Fatalln(err)
	}

	for i := 0; ; i++ {
		if _, err := fmt.Fprintln(r, "hello world", i); err != nil {
			log.Fatalln(err)
		}
		time.Sleep(time.Millisecond)
	}
}
