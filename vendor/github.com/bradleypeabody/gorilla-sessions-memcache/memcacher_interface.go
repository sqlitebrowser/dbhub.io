package gsm

import (
	"github.com/bradfitz/gomemcache/memcache"
)

// Memcacher is the interface gsm uses to interact with the memcache client
type Memcacher interface {
	Get(key string) (val string, flags uint32, cas uint64, err error)
	Set(key, val string, flags, exp uint32, ocas uint64) (cas uint64, err error)
}


// GoMemcacher is a wrapper to the gomemcache client that implements the
// Memcacher interface
type GoMemcacher struct {
  client *memcache.Client
}

// NewGoMemcacher returns a wrapped gomemcache client that implements the
// Memcacher interface
func NewGoMemcacher(c *memcache.Client) *GoMemcacher {
	if c == nil {
		panic("Cannot have nil memcache client")
	}
	return &GoMemcacher{client: c}
}

func (gm *GoMemcacher) Get(key string) (val string, flags uint32, cas uint64, err error) {
	if it, err := gm.client.Get(key); err == nil{
		return string(it.Value), it.Flags, 0, err
	}else{
		return "", 0, 0, err
	}
}


func (gm *GoMemcacher) Set(key, val string, flags, exp uint32, ocas uint64) (cas uint64, err error) {
	err = gm.client.Set(&memcache.Item{Key: key, Value: []byte(val), Expiration: int32(exp), Flags: flags})
	return ocas, err
}
