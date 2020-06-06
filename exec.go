package logger

import (
	"context"

	"github.com/pm-esd/mongodb/v2"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
)

// ExecCloser 将logrus条目写入数据库并关闭数据库
type ExecCloser interface {
	Exec(entry *logrus.Entry) error
}

type defaultExec struct {
	sess     *mongodb.MongoDBClient
	cName    string
	canClose bool
}

// NewExec create an exec instance
func NewExec(sess *mongodb.MongoDBClient, cName string) ExecCloser {
	return &defaultExec{
		sess:     sess,
		cName:    cName,
		canClose: true,
	}
}

// NewExecWithURL create an exec instance
func NewExecWithURL(sess *mongodb.MongoDBClient, cName string) ExecCloser {
	return &defaultExec{
		sess:     sess,
		cName:    cName,
		canClose: true,
	}
}

func (e *defaultExec) Exec(entry *logrus.Entry) error {
	item := make(bson.M)

	for k, v := range entry.Data {
		item[k] = v
	}

	item["level"] = entry.Level
	item["message"] = entry.Message
	item["created"] = entry.Time.Unix()

	ctx := context.Background()
	_, err := e.sess.Collection(e.cName).InsertOne(ctx, item)
	if err != nil {
		return err
	}
	return nil
}
