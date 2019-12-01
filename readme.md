# This package meant to provide a rate limitation function
## Description
This package meant to be used in a distribution system and rely on a redis as a db to sync across different server.\
## Algorythm
rely on sliding window algorythm mentioned in [this link](https://konghq.com/blog/how-to-design-a-scalable-rate-limiting-algorithm/). And it will sync to redis server on a given time.
## How to use
Please refer to mannual [test sample](mannualTest/main.go) on how to use it.
## Refering
This package is based on [go-rate-limiter](https://github.com/manavo/go-rate-limiter)