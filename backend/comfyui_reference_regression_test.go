package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestComfyViewReferenceKeepsImageFilename(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("test-image-content"))
	}))
	defer server.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/comfyui/execute", nil)
	for _, test := range []struct{ suffix, want string }{
		{"/view?filename=keyframe.png&type=output", "keyframe.png"},
		{"/view?filename=frames%2Fkeyframe%20%284%29.png", "keyframe (4).png"},
		{"/view", "view.png"},
	} {
		_, name, err := readComfyImageSource(c, server.URL+test.suffix)
		if err != nil || name != test.want {
			t.Fatalf("%s: name=%q err=%v", test.suffix, name, err)
		}
	}
}

func TestComfyExecutionErrorIncludesActualNodeFailure(t *testing.T) {
	status := map[string]interface{}{"messages": []interface{}{
		[]interface{}{"execution_error", map[string]interface{}{
			"node_id": "11", "node_type": "MiniMaxH3TimelinePlanner", "exception_message": "Reference media not found: view (4)",
		}},
	}}
	want := "ComfyUI 节点 11（MiniMaxH3TimelinePlanner）执行失败：Reference media not found: view (4)"
	if got := comfyExecutionError(status); got != want {
		t.Fatalf("got %q", got)
	}
}
