// Package mpvlib plays media inside KaraokeNino through libmpv: mpv decodes
// video and audio, and hands every video frame to us as pixels (software
// render API) so the screen can draw it like any other picture. There is no
// separate player window.
//
// libmpv is loaded at runtime with dlopen: if it is not installed the
// program still starts and falls back to the external mpv player.
package mpvlib

/*
#cgo linux LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

typedef struct mpv_handle mpv_handle;
typedef struct mpv_render_context mpv_render_context;
typedef struct { int type; void *data; } kn_param;
typedef struct { int event_id; int error; uint64_t reply_userdata; void *data; } kn_event;
typedef struct { const char *name; int format; void *data; } kn_event_property;
typedef struct { int reason; int error; } kn_event_end_file;

static void *kn_lib;
static mpv_handle *(*p_create)(void);
static int (*p_initialize)(mpv_handle *);
static int (*p_set_option_string)(mpv_handle *, const char *, const char *);
static int (*p_command)(mpv_handle *, const char **);
static int (*p_set_property)(mpv_handle *, const char *, int, void *);
static int (*p_observe_property)(mpv_handle *, uint64_t, const char *, int);
static int (*p_get_property)(mpv_handle *, const char *, int, void *);
static kn_event *(*p_wait_event)(mpv_handle *, double);
static void (*p_wakeup)(mpv_handle *);
static void (*p_terminate_destroy)(mpv_handle *);
static const char *(*p_error_string)(int);
static int (*p_render_create)(mpv_render_context **, mpv_handle *, kn_param *);
static int (*p_render_render)(mpv_render_context *, kn_param *);
static uint64_t (*p_render_update)(mpv_render_context *);
static void (*p_render_free)(mpv_render_context *);

#define KN_SYM(var, name) if (!(var = dlsym(kn_lib, name))) return -2;

static int kn_load(void) {
	if (kn_lib) return 0;
	const char *names[] = {"libmpv.so.2", "libmpv.so.1", "libmpv.so", "libmpv.2.dylib", "libmpv.dylib",
		"/opt/homebrew/lib/libmpv.dylib", "/usr/local/lib/libmpv.dylib", NULL};
	for (int i = 0; names[i] && !kn_lib; i++) kn_lib = dlopen(names[i], RTLD_NOW | RTLD_LOCAL);
	if (!kn_lib) return -1;
	KN_SYM(p_create, "mpv_create");
	KN_SYM(p_initialize, "mpv_initialize");
	KN_SYM(p_set_option_string, "mpv_set_option_string");
	KN_SYM(p_command, "mpv_command");
	KN_SYM(p_set_property, "mpv_set_property");
	KN_SYM(p_observe_property, "mpv_observe_property");
	KN_SYM(p_get_property, "mpv_get_property");
	KN_SYM(p_wait_event, "mpv_wait_event");
	KN_SYM(p_wakeup, "mpv_wakeup");
	KN_SYM(p_terminate_destroy, "mpv_terminate_destroy");
	KN_SYM(p_error_string, "mpv_error_string");
	KN_SYM(p_render_create, "mpv_render_context_create");
	KN_SYM(p_render_render, "mpv_render_context_render");
	KN_SYM(p_render_update, "mpv_render_context_update");
	KN_SYM(p_render_free, "mpv_render_context_free");
	return 0;
}

static mpv_handle *kn_create(void) { return p_create(); }
static int kn_initialize(mpv_handle *h) { return p_initialize(h); }
static int kn_option(mpv_handle *h, const char *k, const char *v) { return p_set_option_string(h, k, v); }
static int kn_command(mpv_handle *h, const char **args) { return p_command(h, args); }
static int kn_set_flag(mpv_handle *h, const char *n, int v) { return p_set_property(h, n, 3, &v); }
static int kn_set_double(mpv_handle *h, const char *n, double v) { return p_set_property(h, n, 5, &v); }
static int kn_set_string(mpv_handle *h, const char *n, const char *v) { return p_set_property(h, n, 1, &v); }
static int kn_observe(mpv_handle *h, uint64_t id, const char *n, int fmt) { return p_observe_property(h, id, n, fmt); }
static void kn_wakeup(mpv_handle *h) { p_wakeup(h); }
static int kn_get_int(mpv_handle *h, const char *n, int64_t *v) { return p_get_property(h, n, 4, v); }
static void kn_destroy(mpv_handle *h) { p_terminate_destroy(h); }
static const char *kn_error(int e) { return p_error_string(e); }

// kn_wait returns the event id and fills the fields we use.
static int kn_wait(mpv_handle *h, double timeout, int *err, uint64_t *userdata, int *reason,
                   const char **pname, int *pfmt, double *pdouble, int64_t *pint) {
	kn_event *e = p_wait_event(h, timeout);
	*err = e->error;
	*userdata = e->reply_userdata;
	*reason = -1;
	*pname = NULL;
	*pfmt = 0;
	if (e->event_id == 7 && e->data) {                // END_FILE
		kn_event_end_file *ef = e->data;
		*reason = ef->reason;
		*err = ef->error;
	} else if (e->event_id == 22 && e->data) {        // PROPERTY_CHANGE
		kn_event_property *p = e->data;
		*pname = p->name;
		*pfmt = p->format;
		if (p->format == 5 && p->data) *pdouble = *(double *)p->data;
		if (p->format == 4 && p->data) *pint = *(int64_t *)p->data;
	}
	return e->event_id;
}

static int kn_render_create(mpv_render_context **ctx, mpv_handle *h) {
	kn_param params[] = {{1, "sw"}, {0, NULL}};      // API_TYPE = "sw"
	return p_render_create(ctx, h, params);
}

static int kn_render(mpv_render_context *ctx, int w, int hgt, size_t stride, void *pix) {
	int size[2] = {w, hgt};
	kn_param params[] = {
		{17, size},          // SW_SIZE
		{18, "rgb0"},        // SW_FORMAT
		{19, &stride},       // SW_STRIDE
		{20, pix},           // SW_POINTER
		{0, NULL},
	};
	return p_render_render(ctx, params);
}

static uint64_t kn_render_update(mpv_render_context *ctx) { return p_render_update(ctx); }
static void kn_render_free(mpv_render_context *ctx) { p_render_free(ctx); }

static void *kn_alloc(size_t n) {
	void *p = NULL;
	if (posix_memalign(&p, 64, n)) return NULL;  // 64-byte alignment for fast SIMD paths
	memset(p, 0, n);
	return p;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unsafe"
)

// Event ids we handle (mpv client.h).
const (
	EventNone           = 0
	EventShutdown       = 1
	EventStartFile      = 6
	EventEndFile        = 7
	EventFileLoaded     = 8
	EventVideoReconfig  = 17
	EventPropertyChange = 22
)

// End-of-file reasons.
const (
	EndEOF   = 0
	EndStop  = 2
	EndQuit  = 3
	EndError = 4
)

// Property formats.
const (
	FormatFlag   = 3
	FormatInt64  = 4
	FormatDouble = 5
)

var (
	loadOnce sync.Once
	loadErr  error
)

// Load opens libmpv. It is safe to call many times.
func Load() error {
	loadOnce.Do(func() {
		switch C.kn_load() {
		case 0:
		case -1:
			loadErr = errors.New("libmpv no está instalada (apt install libmpv2)")
		default:
			loadErr = errors.New("libmpv es demasiado vieja (falta la API de render)")
		}
	})
	return loadErr
}

// Event is one mpv event.
type Event struct {
	ID       int
	Err      int
	UserData uint64
	Reason   int // END_FILE
	Name     string
	Format   int
	Double   float64
	Int      int64
}

// Player is one libmpv instance with a software renderer.
type Player struct {
	h   *C.mpv_handle
	ctx *C.mpv_render_context
}

// New creates and starts mpv with options set before initialization.
func New(options [][2]string) (*Player, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	h := C.kn_create()
	if h == nil {
		return nil, errors.New("mpv_create falló")
	}
	p := &Player{h: h}
	for _, kv := range options {
		if err := p.option(kv[0], kv[1]); err != nil {
			// Unknown options (older libmpv) are not fatal.
			_ = err
		}
	}
	if rc := C.kn_initialize(h); rc < 0 {
		C.kn_destroy(h)
		return nil, fmt.Errorf("mpv_initialize: %s", errString(rc))
	}
	var ctx *C.mpv_render_context
	if rc := C.kn_render_create(&ctx, h); rc < 0 {
		C.kn_destroy(h)
		return nil, fmt.Errorf("render por software no disponible: %s", errString(rc))
	}
	p.ctx = ctx
	return p, nil
}

func errString(rc C.int) string { return C.GoString(C.kn_error(rc)) }

func (p *Player) option(k, v string) error {
	ck, cv := C.CString(k), C.CString(v)
	defer C.free(unsafe.Pointer(ck))
	defer C.free(unsafe.Pointer(cv))
	if rc := C.kn_option(p.h, ck, cv); rc < 0 {
		return fmt.Errorf("%s=%s: %s", k, v, errString(rc))
	}
	return nil
}

// Command runs an mpv command, e.g. Command("loadfile", path, "replace").
func (p *Player) Command(args ...string) error {
	cargs := make([]*C.char, len(args)+1)
	for i, a := range args {
		cargs[i] = C.CString(a)
	}
	defer func() {
		for _, c := range cargs[:len(args)] {
			C.free(unsafe.Pointer(c))
		}
	}()
	// The array must live in C memory (cgo pointer rules).
	n := C.size_t(len(cargs)) * C.size_t(unsafe.Sizeof(uintptr(0)))
	arr := (**C.char)(C.malloc(n))
	defer C.free(unsafe.Pointer(arr))
	copy(unsafe.Slice(arr, len(cargs)), cargs)
	if rc := C.kn_command(p.h, arr); rc < 0 {
		return fmt.Errorf("%s: %s", strings.Join(args, " "), errString(rc))
	}
	return nil
}

func (p *Player) SetFlag(name string, v bool) error {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	b := 0
	if v {
		b = 1
	}
	if rc := C.kn_set_flag(p.h, cn, C.int(b)); rc < 0 {
		return fmt.Errorf("%s: %s", name, errString(rc))
	}
	return nil
}

func (p *Player) SetDouble(name string, v float64) error {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	if rc := C.kn_set_double(p.h, cn, C.double(v)); rc < 0 {
		return fmt.Errorf("%s: %s", name, errString(rc))
	}
	return nil
}

func (p *Player) SetString(name, v string) error {
	cn, cv := C.CString(name), C.CString(v)
	defer C.free(unsafe.Pointer(cn))
	defer C.free(unsafe.Pointer(cv))
	if rc := C.kn_set_string(p.h, cn, cv); rc < 0 {
		return fmt.Errorf("%s: %s", name, errString(rc))
	}
	return nil
}

// GetInt reads an integer property now (e.g. "dwidth"); ok is false if
// it is not available (no video).
func (p *Player) GetInt(name string) (int64, bool) {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	var v C.int64_t
	if rc := C.kn_get_int(p.h, cn, &v); rc < 0 {
		return 0, false
	}
	return int64(v), true
}

// Observe asks for PROPERTY_CHANGE events of name.
func (p *Player) Observe(id uint64, name string, format int) error {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	if rc := C.kn_observe(p.h, C.uint64_t(id), cn, C.int(format)); rc < 0 {
		return fmt.Errorf("observe %s: %s", name, errString(rc))
	}
	return nil
}

// WaitEvent blocks up to timeout seconds (-1 = forever) for the next event.
func (p *Player) WaitEvent(timeout float64) Event {
	var err, reason, fmtv C.int
	var ud C.uint64_t
	var name *C.char
	var d C.double
	var i C.int64_t
	id := C.kn_wait(p.h, C.double(timeout), &err, &ud, &reason, &name, &fmtv, &d, &i)
	ev := Event{ID: int(id), Err: int(err), UserData: uint64(ud), Reason: int(reason), Format: int(fmtv), Double: float64(d), Int: int64(i)}
	if name != nil {
		ev.Name = C.GoString(name)
	}
	return ev
}

// Wakeup interrupts a blocking WaitEvent.
func (p *Player) Wakeup() { C.kn_wakeup(p.h) }

// FrameReady reports whether a new video frame should be rendered.
func (p *Player) FrameReady() bool {
	return uint64(C.kn_render_update(p.ctx))&1 != 0
}

// Frame is a 64-byte aligned RGB buffer in C memory that mpv renders into.
type Frame struct {
	W, H   int
	Stride int
	ptr    unsafe.Pointer
	Pix    []byte // view of the C buffer (alpha bytes are garbage: "rgb0")
}

// NewFrame allocates a frame of w×h pixels. Free it with Free.
func NewFrame(w, h int) *Frame {
	stride := (w*4 + 63) &^ 63
	ptr := C.kn_alloc(C.size_t(stride * h))
	if ptr == nil {
		return nil
	}
	return &Frame{W: w, H: h, Stride: stride, ptr: ptr, Pix: unsafe.Slice((*byte)(ptr), stride*h)}
}

func (f *Frame) Free() {
	if f != nil && f.ptr != nil {
		C.free(f.ptr)
		f.ptr, f.Pix = nil, nil
	}
}

// Render draws the current video frame into f.
func (p *Player) Render(f *Frame) error {
	if rc := C.kn_render(p.ctx, C.int(f.W), C.int(f.H), C.size_t(f.Stride), f.ptr); rc < 0 {
		return fmt.Errorf("render: %s", errString(rc))
	}
	return nil
}

// Destroy stops mpv and frees everything.
func (p *Player) Destroy() {
	if p.ctx != nil {
		C.kn_render_free(p.ctx)
		p.ctx = nil
	}
	if p.h != nil {
		C.kn_destroy(p.h)
		p.h = nil
	}
}
