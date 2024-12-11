module github.com/sqlitebrowser/dbhub.io

go 1.18

replace (
	github.com/Sirupsen/logrus v1.0.5 => github.com/sirupsen/logrus v1.0.5
	github.com/Sirupsen/logrus v1.3.0 => github.com/sirupsen/logrus v1.3.0
	github.com/Sirupsen/logrus v1.4.0 => github.com/sirupsen/logrus v1.4.0
	github.com/Sirupsen/logrus v1.5.0 => github.com/sirupsen/logrus v1.5.0
	github.com/Sirupsen/logrus v1.6.0 => github.com/sirupsen/logrus v1.6.0
)

require (
	github.com/BurntSushi/toml v1.3.2
	github.com/NYTimes/gziphandler v1.1.1
	github.com/aquilax/truncate v1.0.0
	github.com/bradfitz/gomemcache v0.0.0-20230905024940-24af94b03874
	github.com/bradleypeabody/gorilla-sessions-memcache v0.0.0-20181103040241-659414f458e1
	github.com/docker/go-units v0.5.0
	github.com/gin-contrib/cors v1.7.2
	github.com/gin-contrib/gzip v1.0.1
	github.com/gin-gonic/gin v1.9.1
	github.com/go-playground/validator/v10 v10.20.0
	github.com/golang-migrate/migrate/v4 v4.17.1
	github.com/gwenn/gosqlite v0.0.0-20200521090053-24878be1a237
	github.com/jackc/pgx/v5 v5.5.5
	github.com/minio/minio-go v6.0.14+incompatible
	github.com/pkg/errors v0.9.1
	github.com/smtp2go-oss/smtp2go-go v1.0.3
	github.com/sqlitebrowser/github_flavored_markdown v0.0.0-20190120045821-b8cf8f054e47
	golang.org/x/oauth2 v0.20.0
	gorm.io/driver/postgres v1.5.7
	gorm.io/gorm v1.25.10
)

require (
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/bytedance/sonic v1.11.6 // indirect
	github.com/bytedance/sonic/loader v0.1.1 // indirect
	github.com/cloudwego/base64x v0.1.4 // indirect
	github.com/cloudwego/iasm v0.2.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/gin-contrib/sse v0.1.0 // indirect
	github.com/go-ini/ini v1.56.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/gorilla/css v1.0.0 // indirect
	github.com/gorilla/securecookie v1.1.1 // indirect
	github.com/gorilla/sessions v1.2.0 // indirect
	github.com/gwenn/yacr v0.0.0-20200112083327-bbe82c1f4d60 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jackc/pgerrcode v0.0.0-20220416144525-469b46aa5efa // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/cpuid/v2 v2.2.7 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/memcachier/mc v2.0.1+incompatible // indirect
	github.com/microcosm-cc/bluemonday v1.0.16 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pelletier/go-toml/v2 v2.2.2 // indirect
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	github.com/russross/blackfriday v2.0.0+incompatible // indirect
	github.com/sergi/go-diff v1.1.0 // indirect
	github.com/shurcooL/github_flavored_markdown v0.0.0-20181002035957-2122de532470 // indirect
	github.com/shurcooL/go v0.0.0-20190704215121-7189cc372560 // indirect
	github.com/shurcooL/go-goon v0.0.0-20170922171312-37c2f522c041 // indirect
	github.com/shurcooL/highlight_diff v0.0.0-20181222201841-111da2e7d480 // indirect
	github.com/shurcooL/highlight_go v0.0.0-20191220051317-782971ddf21b // indirect
	github.com/shurcooL/octicon v0.0.0-20191102190552-cbb32d6a785c // indirect
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	github.com/smartystreets/goconvey v1.8.0 // indirect
	github.com/sourcegraph/annotate v0.0.0-20160123013949-f4cad6c6324d // indirect
	github.com/sourcegraph/syntaxhighlight v0.0.0-20170531221838-bd320f5d308e // indirect
	github.com/sqlitebrowser/blackfriday v9.0.0+incompatible // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.2.12 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	golang.org/x/arch v0.7.0 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/net v0.24.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/protobuf v1.34.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
