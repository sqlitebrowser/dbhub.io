package dkim

import (
	"crypto"
	_ "crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	minRequired = []string{
		VersionKey,
		AlgorithmKey,
		DomainKey,
		SelectorKey,
		CanonicalizationKey,
		QueryMethodKey,
		TimestampKey,
	}
	keyOrder = []string{
		VersionKey,
		AlgorithmKey,
		CanonicalizationKey,
		DomainKey,
		QueryMethodKey,
		SelectorKey,
		TimestampKey,
		BodyHashKey,
		FieldsKey,
		CopiedFieldsKey,
		AUIDKey,
		BodyLengthKey,
		SignatureDataKey,
	}
)

type Conf map[string]string

const (
	VersionKey          = "v"
	AlgorithmKey        = "a"
	DomainKey           = "d"
	SelectorKey         = "s"
	CanonicalizationKey = "c"
	QueryMethodKey      = "q"
	BodyLengthKey       = "l"
	TimestampKey        = "t"
	ExpireKey           = "x"
	FieldsKey           = "h"
	BodyHashKey         = "bh"
	SignatureDataKey    = "b"
	AUIDKey             = "i"
	CopiedFieldsKey     = "z"
)

const (
	AlgorithmSHA256         = "rsa-sha256"
	DefaultVersion          = "1"
	DefaultCanonicalization = "relaxed/simple"
	DefaultQueryMethod      = "dns/txt"
)

func NewConf(domain string, selector string) (Conf, error) {
	if domain == "" {
		return nil, fmt.Errorf("domain invalid")
	}
	if selector == "" {
		return nil, fmt.Errorf("selector invalid")
	}
	return Conf{
		VersionKey:          DefaultVersion,
		AlgorithmKey:        AlgorithmSHA256,
		DomainKey:           domain,
		SelectorKey:         selector,
		CanonicalizationKey: DefaultCanonicalization,
		QueryMethodKey:      DefaultQueryMethod,
		TimestampKey:        strconv.FormatInt(time.Now().Unix(), 10),
		FieldsKey:           Empty,
		BodyHashKey:         Empty,
		SignatureDataKey:    Empty,
	}, nil
}

func (c Conf) Validate() error {
	for _, key := range minRequired {
		if _, ok := c[key]; !ok {
			return fmt.Errorf("key '%s' missing", key)
		}
	}
	return nil
}

func (c Conf) Algorithm() string {
	if algorithm := c[AlgorithmKey]; algorithm != Empty {
		return algorithm
	}
	return AlgorithmSHA256
}

func (c Conf) Hash() crypto.Hash {
	if c.Algorithm() == AlgorithmSHA256 {
		return crypto.SHA256
	}
	panic("algorithm not implemented")
}

func (c Conf) RelaxedHeader() bool {
	if strings.HasPrefix(strings.ToLower(c[CanonicalizationKey]), "relaxed") {
		return true
	}
	return false
}

func (c Conf) RelaxedBody() bool {
	if strings.HasSuffix(strings.ToLower(c[CanonicalizationKey]), "/relaxed") {
		return true
	}
	return false
}

func (c Conf) String() string {
	pairs := make([]string, 0, len(keyOrder))
	for _, key := range keyOrder {
		if value, ok := c[key]; ok {
			pairs = append(pairs, fmt.Sprintf("%s=%s", key, value))
		}
	}
	return strings.Join(pairs, "; ")
}
