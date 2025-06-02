package utils

import (
	"context"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	etcdUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/etcd"
)

func EtcdLocker(c *clientv3.Client, lockName string, lockTimeout time.Duration) (func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()
	mutex, session, err := etcdUtils.Lock(ctx, c, lockName)
	if err != nil {
		return nil, err
	}
	return func() {
		etcdUtils.UnLock(context.TODO(), mutex, session)
	}, nil
}
