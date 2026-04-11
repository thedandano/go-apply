package port

import "github.com/thedandano/go-apply/internal/model"

type JDCacheRepository interface {
	Get(url string) (rawText string, jd model.JDData, found bool)
	Put(url string, rawText string, jd model.JDData) error
	Update(url string, jd model.JDData) error
}
