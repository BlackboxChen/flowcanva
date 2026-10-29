package main

import (
	"github.com/gin-gonic/gin"
	"strings"
)

func (h *Handler) RestoreAsset(c *gin.Context) {
	if _, err := h.db.Exec(`DELETE FROM asset_trash WHERE asset_id=?`, c.Param("id")); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.Status(204)
}
func (h *Handler) AssetReferences(c *gin.Context) {
	var url string
	if err := h.db.QueryRow(`SELECT url FROM assets WHERE id=?`, c.Param("id")).Scan(&url); err != nil {
		c.Status(404)
		return
	}
	rows, err := h.db.Query(`SELECT n.id,n.canvas_id,c.name,n.content,n.config FROM nodes n JOIN canvases c ON c.id=n.canvas_id`)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	refs := []map[string]string{}
	for rows.Next() {
		var id, cid, name, content, config string
		if rows.Scan(&id, &cid, &name, &content, &config) == nil && (strings.Contains(content, url) || strings.Contains(config, url)) {
			refs = append(refs, map[string]string{"node_id": id, "canvas_id": cid, "canvas_name": name})
		}
	}
	c.JSON(200, refs)
}
func (h *Handler) CanvasProject(c *gin.Context) {
	cid := c.Param("cid")
	if c.Request.Method == "PUT" {
		var body struct {
			Project string `json:"project"`
		}
		if c.ShouldBindJSON(&body) != nil || len(body.Project) > 200 {
			c.JSON(400, gin.H{"error": "项目名称无效"})
			return
		}
		if _, err := h.db.Exec(`INSERT INTO canvas_projects VALUES(?,?) ON CONFLICT(canvas_id) DO UPDATE SET project=excluded.project`, cid, strings.TrimSpace(body.Project)); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	var project string
	_ = h.db.QueryRow(`SELECT project FROM canvas_projects WHERE canvas_id=?`, cid).Scan(&project)
	c.JSON(200, gin.H{"project": project})
}
