package config

import (
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUseViper(t *testing.T) {
	cfg := viper.New()
	cfg.Set("mode", "production")
	cfg.Set("couchdb.host", "db")
	cfg.Set("couchdb.port", 1234)

	UseViper(cfg)

	assert.Equal(t, Production, GetConfig().Mode)
	assert.Equal(t, "http://db:1234/", CouchURL())
}
