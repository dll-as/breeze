package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nelthaarion/breeze/events"
)

type UserCreated struct {
	ID uint64
}

func main() {

	fmt.Println("================================")
	fmt.Println(" BREEZE EVENT STRESS VALIDATOR ")
	fmt.Println("================================")

	var received atomic.Uint64

	var wg sync.WaitGroup

	// Permanent listener
	events.On(
		UserCreated{},
		func(
			ctx *events.Context,
			e UserCreated,
		) error {

			received.Add(1)

			return nil
		},
	)

	start := time.Now()

	// Dynamic register/unregister workers
	for i := 0; i < 100; i++ {

		wg.Add(1)

		go func(id int) {

			defer wg.Done()

			for j := 0; j < 10000; j++ {

				sub := events.On(
					UserCreated{},
					func(
						ctx *events.Context,
						e UserCreated,
					) error {

						return nil
					},
				)

				// immediately remove
				sub.Unsubscribe()

				// emit while registry changes
				events.Emit(
					UserCreated{
						ID: uint64(id*10000 + j),
					},
				)
			}

		}(i)
	}

	// Heavy emitters
	for i := 0; i < 100; i++ {

		wg.Add(1)

		go func(id int) {

			defer wg.Done()

			for j := 0; j < 100000; j++ {

				events.Emit(
					UserCreated{
						ID: uint64(j),
					},
				)

			}

		}(i)

	}

	wg.Wait()

	elapsed := time.Since(start)

	fmt.Println()
	fmt.Println("========= RESULT =========")

	fmt.Println(
		"Received:",
		received.Load(),
	)

	fmt.Println(
		"Duration:",
		elapsed,
	)

	fmt.Println()
	fmt.Println("PASS: no panic")
	fmt.Println("PASS: no deadlock")
	fmt.Println("PASS: concurrent register/unregister")
	fmt.Println("PASS: concurrent dispatch")

}
