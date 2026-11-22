package tsx

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/web"
)

func init() {
	moduleTable["nsgo/web"] = []export{faceExport("Head", pageHeadFace)}
}

func pageHeadFace(p *props) core.Node {
	h := web.Head{}
	if p.has("title") {
		h.TitleFunc = p.strFunc("title")
	}
	if p.has("description") {
		h.DescriptionFunc = p.strFunc("description")
	}
	return h
}
