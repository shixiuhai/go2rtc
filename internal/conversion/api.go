package conversion

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/flv"
)

type AddTaskRequest struct {
	Path     string `json:"path"`
	CameraID string `json:"cameraId"`
}

type DeleteTaskRequest struct {
	TaskID string `json:"taskId"`
}

type KeepAliveRequest struct {
	TaskID string `json:"taskId"`
}

func apiAddTask(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		setCorsHeaders(w)
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}

	if r.Body == nil {
		writeJSON(w, 400, "empty body")
		return
	}

	var req AddTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, "invalid json")
		return
	}

	if req.Path == "" && req.CameraID == "" {
		writeJSON(w, 400, "need path or cameraId")
		return
	}

	rtspPath := req.Path
	if rtspPath == "" {
		rtspPath = req.CameraID
	}

	taskID := GenerateTaskID(rtspPath)
	flvURL := "/flv/" + taskID + ".flv"

	existing := manager.GetTaskInfo(taskID)
	if existing != nil {
		writeJSON(w, 200, map[string]any{
			"code":   200,
			"result": "rtsp 流转换成功",
			"path":   rtspPath,
			"taskId": taskID,
			"flvUrl": existing.FLVURL,
		})
		return
	}

	success := manager.AddTask(rtspPath, flvURL, taskID)
	if !success {
		writeJSON(w, 500, map[string]any{
			"code":   500,
			"result": "流数量已达上限 (" + strconv.Itoa(manager.maxStreams) + ")，转换失败",
		})
		return
	}

	writeJSON(w, 200, map[string]any{
		"code":   200,
		"result": "rtsp 流转换成功",
		"path":   rtspPath,
		"taskId": taskID,
		"flvUrl": flvURL,
	})
}

func apiDeleteTask(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		setCorsHeaders(w)
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}

	if r.Body == nil {
		writeJSON(w, 400, "empty body")
		return
	}

	var req DeleteTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, "invalid json")
		return
	}

	if req.TaskID == "" {
		writeJSON(w, 400, "taskId required")
		return
	}

	if manager.RemoveTask(req.TaskID) {
		writeJSON(w, 200, map[string]any{
			"code":   200,
			"result": "关闭" + req.TaskID + "任务成功",
		})
	} else {
		writeJSON(w, 500, map[string]any{
			"code":   500,
			"result": "任务不存在或已关闭",
		})
	}
}

func apiKeepAlive(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		setCorsHeaders(w)
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}

	if r.Body == nil {
		writeJSON(w, 400, "empty body")
		return
	}

	var req KeepAliveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, "invalid json")
		return
	}

	if req.TaskID == "" {
		writeJSON(w, 400, "taskId required")
		return
	}

	if manager.KeepAlive(req.TaskID) {
		writeJSON(w, 200, map[string]any{
			"code":   200,
			"result": req.TaskID + "保活成功",
		})
	} else {
		writeJSON(w, 500, map[string]any{
			"code":   500,
			"result": "任务不存在",
		})
	}
}

func apiListTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		setCorsHeaders(w)
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != "POST" && r.Method != "GET" {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}

	tasks := manager.ListTasks()
	taskLiveDict := make(map[string]int64, len(tasks))
	for tid, info := range tasks {
		taskLiveDict[tid] = info.Expires
	}

	writeJSON(w, 200, map[string]any{
		"code":         200,
		"result":       "查看转换流列表成功",
		"taskDict":     tasks,
		"taskLiveDict": taskLiveDict,
		"num":          len(tasks),
	})
}

func apiHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		setCorsHeaders(w)
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}

	tasks := manager.ListTasks()
	writeJSON(w, 200, map[string]any{
		"status":  "ok",
		"streams": len(tasks),
	})
}

func apiFLV(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}

	taskID := r.URL.Query().Get("id")
	if taskID == "" {
		taskID = strings.TrimPrefix(r.URL.Path, "/flv/")
		taskID = strings.TrimSuffix(taskID, ".flv")
	}

	info := manager.GetTaskInfo(taskID)
	if info == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	stream := streams.Get(info.StreamName)
	if stream == nil {
		http.Error(w, "stream not found", http.StatusNotFound)
		return
	}

	cons := flv.NewConsumer()
	cons.WithRequest(r)

	if err := stream.AddConsumer(cons); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "video/x-flv")
	h.Set("Cache-Control", "no-cache")
	h.Set("Access-Control-Allow-Origin", "*")

	_, _ = cons.WriteTo(w)

	stream.RemoveConsumer(cons)
}

func setCorsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	setCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
