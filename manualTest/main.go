package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/olivemz/limiter"
)

func final(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("OK"))
}

func main() {
	redisPool := &redis.Pool{
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", "127.0.0.1:6379")
			if err != nil {
				return nil, err
			}
			return c, err
		},
	}
	mux := http.NewServeMux()

	rl := limiter.New(redisPool, "requests", 23, time.Minute, 2*time.Second)
	rlErr := rl.Init()

	if rlErr != nil {
		fmt.Println(rlErr)
		panic("rate limit module faled to load")
	}
	finalHandler := http.HandlerFunc(final)
	mux.Handle("/", rl.MiddleWareHandler(finalHandler))
	http.ListenAndServe(":3000", mux)
}
