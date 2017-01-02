package main

import (
	"time"
)

// Configuration file
type tomlConfig struct {
	Cache cacheInfo
	Minio minioInfo
	Pg    pgInfo
	Web   webInfo
}

// Memcached connection parameters
type cacheInfo struct {
	Server string
}

// Minio connection parameters
type minioInfo struct {
	Server    string
	AccessKey string `toml:"access_key"`
	Secret    string
	HTTPS     bool
}

// PostgreSQL connection parameters
type pgInfo struct {
	Server   string
	Port     int
	Username string
	Password string
	Database string
}

type webInfo struct {
	Server         string
	Certificate    string
	CertificateKey string `toml:"certificate_key"`
	RequestLog     string `toml:"request_log"`
}

type dataValue struct {
	Name  string
	Type  ValType
	Value interface{}
}
type dataRow []dataValue
type dbInfo struct {
	Database     string
	Tables       []string
	Watchers     int
	Stars        int
	Forks        int
	Discussions  int
	MRs          int
	Description  string
	Updates      int
	Branches     int
	Releases     int
	Contributors int
	Readme       string
	DateCreated  time.Time
	LastModified time.Time
	Public       bool
	Size         int
	Version      int
}

type metaInfo struct {
	Protocol     string
	Server       string
	Title        string
	Username     string
	Database     string
	LoggedInUser string
}

type sqliteDBinfo struct {
	Info     dbInfo
	MaxRows  int
	MinioBkt string
	MinioId  string
}

type sqliteRecordSet struct {
	Tablename string
	ColNames  []string
	ColCount  int
	RowCount  int
	TotalRows int
	Records   []dataRow
}

type whereClause struct {
	Column string
	Type   string
	Value  string
}
