package redis

import (
	"github.com/redis/rueidis"
)

func Connect() (rueidis.Client, error) {
	client, err := rueidis.NewClient(rueidis.ClientOption{InitAddress: []string{"127.0.0.1:6379"}})
	if err != nil {
		return nil, err
	}
	return client, nil
}
