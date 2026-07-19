//go:build linux && cgo

package gen

/*
#cgo linux LDFLAGS: -ldl

#include <dlfcn.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef int32_t cl_int;
typedef uint32_t cl_uint;
typedef uint64_t cl_ulong;
typedef cl_ulong cl_device_type;
typedef cl_ulong cl_mem_flags;
typedef cl_ulong cl_command_queue_properties;
typedef uint32_t cl_bool;
typedef uint32_t cl_device_info;
typedef uint32_t cl_program_build_info;
typedef struct _cl_platform_id *cl_platform_id;
typedef struct _cl_device_id *cl_device_id;
typedef struct _cl_context *cl_context;
typedef struct _cl_command_queue *cl_command_queue;
typedef struct _cl_mem *cl_mem;
typedef struct _cl_program *cl_program;
typedef struct _cl_kernel *cl_kernel;

#define CL_SUCCESS 0
#define CL_TRUE 1
#define CL_DEVICE_TYPE_GPU (1ULL << 2)
#define CL_MEM_READ_WRITE (1ULL << 0)
#define CL_MEM_WRITE_ONLY (1ULL << 1)
#define CL_MEM_READ_ONLY (1ULL << 2)
#define CL_MEM_COPY_HOST_PTR (1ULL << 5)
#define CL_DEVICE_NAME 0x102B
#define CL_DEVICE_VENDOR 0x102C
#define CL_DEVICE_EXTENSIONS 0x1030
#define CL_DEVICE_DOUBLE_FP_CONFIG 0x1032
#define CL_PROGRAM_BUILD_LOG 0x1183

static void *vg_cl_lib;
static char vg_cl_error[1024];

static cl_int (*p_clGetPlatformIDs)(cl_uint, cl_platform_id *, cl_uint *);
static cl_int (*p_clGetDeviceIDs)(cl_platform_id, cl_device_type, cl_uint, cl_device_id *, cl_uint *);
static cl_int (*p_clGetDeviceInfo)(cl_device_id, cl_device_info, size_t, void *, size_t *);
static cl_context (*p_clCreateContext)(const intptr_t *, cl_uint, const cl_device_id *, void *, void *, cl_int *);
static cl_command_queue (*p_clCreateCommandQueue)(cl_context, cl_device_id, cl_command_queue_properties, cl_int *);
static cl_program (*p_clCreateProgramWithSource)(cl_context, cl_uint, const char **, const size_t *, cl_int *);
static cl_int (*p_clBuildProgram)(cl_program, cl_uint, const cl_device_id *, const char *, void *, void *);
static cl_int (*p_clGetProgramBuildInfo)(cl_program, cl_device_id, cl_program_build_info, size_t, void *, size_t *);
static cl_kernel (*p_clCreateKernel)(cl_program, const char *, cl_int *);
static cl_mem (*p_clCreateBuffer)(cl_context, cl_mem_flags, size_t, void *, cl_int *);
static cl_int (*p_clSetKernelArg)(cl_kernel, cl_uint, size_t, const void *);
static cl_int (*p_clEnqueueWriteBuffer)(cl_command_queue, cl_mem, cl_bool, size_t, size_t, const void *, cl_uint, const void *, void *);
static cl_int (*p_clEnqueueReadBuffer)(cl_command_queue, cl_mem, cl_bool, size_t, size_t, void *, cl_uint, const void *, void *);
static cl_int (*p_clEnqueueNDRangeKernel)(cl_command_queue, cl_kernel, cl_uint, const size_t *, const size_t *, const size_t *, cl_uint, const void *, void *);
static cl_int (*p_clFinish)(cl_command_queue);
static cl_int (*p_clReleaseMemObject)(cl_mem);
static cl_int (*p_clReleaseKernel)(cl_kernel);
static cl_int (*p_clReleaseProgram)(cl_program);
static cl_int (*p_clReleaseCommandQueue)(cl_command_queue);
static cl_int (*p_clReleaseContext)(cl_context);

#define VG_LOAD_SYMBOL(name) do { \
    p_##name = dlsym(vg_cl_lib, #name); \
    if (!p_##name) { \
        snprintf(vg_cl_error, sizeof(vg_cl_error), "missing OpenCL symbol %s", #name); \
        dlclose(vg_cl_lib); \
        vg_cl_lib = NULL; \
        return -1; \
    } \
} while (0)

static int vg_cl_load(const char *explicit_path) {
    if (vg_cl_lib) return 0;
    if (explicit_path && explicit_path[0]) {
        vg_cl_lib = dlopen(explicit_path, RTLD_NOW | RTLD_LOCAL);
    } else {
        vg_cl_lib = dlopen("libOpenCL.so.1", RTLD_NOW | RTLD_LOCAL);
        if (!vg_cl_lib) vg_cl_lib = dlopen("libOpenCL.so", RTLD_NOW | RTLD_LOCAL);
    }
    if (!vg_cl_lib) {
        const char *message = dlerror();
        snprintf(vg_cl_error, sizeof(vg_cl_error), "%s", message ? message : "unable to load OpenCL");
        return -1;
    }
    VG_LOAD_SYMBOL(clGetPlatformIDs);
    VG_LOAD_SYMBOL(clGetDeviceIDs);
    VG_LOAD_SYMBOL(clGetDeviceInfo);
    VG_LOAD_SYMBOL(clCreateContext);
    VG_LOAD_SYMBOL(clCreateCommandQueue);
    VG_LOAD_SYMBOL(clCreateProgramWithSource);
    VG_LOAD_SYMBOL(clBuildProgram);
    VG_LOAD_SYMBOL(clGetProgramBuildInfo);
    VG_LOAD_SYMBOL(clCreateKernel);
    VG_LOAD_SYMBOL(clCreateBuffer);
    VG_LOAD_SYMBOL(clSetKernelArg);
    VG_LOAD_SYMBOL(clEnqueueWriteBuffer);
    VG_LOAD_SYMBOL(clEnqueueReadBuffer);
    VG_LOAD_SYMBOL(clEnqueueNDRangeKernel);
    VG_LOAD_SYMBOL(clFinish);
    VG_LOAD_SYMBOL(clReleaseMemObject);
    VG_LOAD_SYMBOL(clReleaseKernel);
    VG_LOAD_SYMBOL(clReleaseProgram);
    VG_LOAD_SYMBOL(clReleaseCommandQueue);
    VG_LOAD_SYMBOL(clReleaseContext);
    return 0;
}

static const char *vg_cl_last_error(void) { return vg_cl_error; }

static cl_int vg_cl_pick_gpu(int platform_index, int device_index, cl_platform_id *selected_platform, cl_device_id *selected_device) {
    cl_uint platform_count = 0;
    cl_int status = p_clGetPlatformIDs(0, NULL, &platform_count);
    if (status != CL_SUCCESS) return status;
    if (platform_count == 0) return -1001;
    cl_platform_id *platforms = calloc(platform_count, sizeof(cl_platform_id));
    if (!platforms) return -6;
    status = p_clGetPlatformIDs(platform_count, platforms, NULL);
    if (status != CL_SUCCESS) { free(platforms); return status; }
    if (platform_index < 0) platform_index = 0;
    if ((cl_uint)platform_index >= platform_count) { free(platforms); return -32; }
    cl_platform_id platform = platforms[platform_index];
    free(platforms);

    cl_uint device_count = 0;
    status = p_clGetDeviceIDs(platform, CL_DEVICE_TYPE_GPU, 0, NULL, &device_count);
    if (status != CL_SUCCESS) return status;
    if (device_count == 0) return -1;
    cl_device_id *devices = calloc(device_count, sizeof(cl_device_id));
    if (!devices) return -6;
    status = p_clGetDeviceIDs(platform, CL_DEVICE_TYPE_GPU, device_count, devices, NULL);
    if (status != CL_SUCCESS) { free(devices); return status; }
    if (device_index < 0) device_index = 0;
    if ((cl_uint)device_index >= device_count) { free(devices); return -33; }
    *selected_platform = platform;
    *selected_device = devices[device_index];
    free(devices);
    return CL_SUCCESS;
}

static char *vg_cl_device_string(cl_device_id device, cl_device_info param) {
    size_t size = 0;
    if (p_clGetDeviceInfo(device, param, 0, NULL, &size) != CL_SUCCESS || size == 0) return NULL;
    char *value = calloc(size + 1, 1);
    if (!value) return NULL;
    if (p_clGetDeviceInfo(device, param, size, value, NULL) != CL_SUCCESS) { free(value); return NULL; }
    return value;
}

static cl_ulong vg_cl_device_ulong(cl_device_id device, cl_device_info param, cl_int *status) {
    cl_ulong value = 0;
    *status = p_clGetDeviceInfo(device, param, sizeof(value), &value, NULL);
    return value;
}

static cl_context vg_cl_create_context(cl_device_id device, cl_int *status) {
    return p_clCreateContext(NULL, 1, &device, NULL, NULL, status);
}
static cl_command_queue vg_cl_create_queue(cl_context context, cl_device_id device, cl_int *status) {
    return p_clCreateCommandQueue(context, device, 0, status);
}
static cl_program vg_cl_create_program(cl_context context, const char *source, size_t length, cl_int *status) {
    return p_clCreateProgramWithSource(context, 1, &source, &length, status);
}
static cl_int vg_cl_build_program(cl_program program, cl_device_id device, const char *options) {
    return p_clBuildProgram(program, 1, &device, options, NULL, NULL);
}
static char *vg_cl_build_log(cl_program program, cl_device_id device) {
    size_t size = 0;
    if (p_clGetProgramBuildInfo(program, device, CL_PROGRAM_BUILD_LOG, 0, NULL, &size) != CL_SUCCESS) return NULL;
    char *log = calloc(size + 1, 1);
    if (!log) return NULL;
    if (p_clGetProgramBuildInfo(program, device, CL_PROGRAM_BUILD_LOG, size, log, NULL) != CL_SUCCESS) { free(log); return NULL; }
    return log;
}
static cl_kernel vg_cl_create_kernel(cl_program program, const char *name, cl_int *status) {
    return p_clCreateKernel(program, name, status);
}
static cl_mem vg_cl_create_buffer(cl_context context, cl_mem_flags flags, size_t size, void *host, cl_int *status) {
    return p_clCreateBuffer(context, flags, size, host, status);
}
static cl_int vg_cl_set_mem(cl_kernel kernel, cl_uint index, cl_mem memory) {
    return p_clSetKernelArg(kernel, index, sizeof(memory), &memory);
}
static cl_int vg_cl_write(cl_command_queue queue, cl_mem memory, size_t size, const void *data) {
    return p_clEnqueueWriteBuffer(queue, memory, CL_TRUE, 0, size, data, 0, NULL, NULL);
}
static cl_int vg_cl_read(cl_command_queue queue, cl_mem memory, size_t size, void *data) {
    return p_clEnqueueReadBuffer(queue, memory, CL_TRUE, 0, size, data, 0, NULL, NULL);
}
static cl_int vg_cl_run_1d(cl_command_queue queue, cl_kernel kernel, size_t global_size) {
    return p_clEnqueueNDRangeKernel(queue, kernel, 1, NULL, &global_size, NULL, 0, NULL, NULL);
}
static cl_int vg_cl_finish(cl_command_queue queue) { return p_clFinish(queue); }
static void vg_cl_release_mem(cl_mem value) { if (value) p_clReleaseMemObject(value); }
static void vg_cl_release_kernel(cl_kernel value) { if (value) p_clReleaseKernel(value); }
static void vg_cl_release_program(cl_program value) { if (value) p_clReleaseProgram(value); }
static void vg_cl_release_queue(cl_command_queue value) { if (value) p_clReleaseCommandQueue(value); }
static void vg_cl_release_context(cl_context value) { if (value) p_clReleaseContext(value); }
*/
import "C"

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

const (
	defaultOpenCLClimateBatch = 16_384
	openCLPerlinParamCount    = 8
	openCLDensityColumns      = 25
	openCLDensityInputs       = openCLDensityColumns * 3
	openCLMaxDensityCorners   = 5 * 5 * 49
)

//go:embed opencl_overworld.cl
var openCLOverworldBaseSource string

var openCLLoaderMu sync.Mutex

func configureOpenCLICD(explicit string) error {
	if explicit == "" && (os.Getenv("OCL_ICD_FILENAMES") != "" || os.Getenv("OCL_ICD_VENDORS") != "") {
		return nil
	}
	path := explicit
	if path == "" && !hasOpenCLVendorFile("/etc/OpenCL/vendors") {
		const nixNVIDIAICD = "/run/opengl-driver/lib/libnvidia-opencl.so.1"
		if info, err := os.Stat(nixNVIDIAICD); err == nil && !info.IsDir() {
			path = nixNVIDIAICD
		}
	}
	if path == "" {
		return nil
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		if err == nil {
			err = fmt.Errorf("path is a directory")
		}
		return fmt.Errorf("OpenCL ICD library %q: %w", path, err)
	}
	return os.Setenv("OCL_ICD_FILENAMES", path)
}

func hasOpenCLVendorFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".icd" {
			return true
		}
	}
	return false
}

type openCLAccelerator struct {
	mu     sync.Mutex
	closed bool
	info   AcceleratorInfo
	noises *NoiseRegistry

	device  C.cl_device_id
	context C.cl_context
	queue   C.cl_command_queue
	program C.cl_program
	density C.cl_kernel
	climate C.cl_kernel
	biome   C.cl_kernel

	staticMem  []C.cl_mem
	densityIn  C.cl_mem
	densityOut C.cl_mem
	chunkMeta  C.cl_mem
	climateIn  C.cl_mem
	climateOut C.cl_mem
	biomeIn    C.cl_mem
	biomeOut   C.cl_mem

	climateBatch  int
	columnScratch []int32
	biomeScratch  []int32
	columnStarts  []int
	climateData   []float64
}

// NewOpenCLAccelerator creates a strict-float64 OpenCL backend. Kernel source
// is compiled once and all seed-specific noise tables remain resident on the
// device for the accelerator's lifetime.
func NewOpenCLAccelerator(noises *NoiseRegistry, cfg OpenCLConfig) (ComputeAccelerator, error) {
	if noises == nil {
		return nil, fmt.Errorf("%w: nil noise registry", ErrAcceleratorUnavailable)
	}
	batch := cfg.ClimateBatchSize
	if batch < 256 {
		batch = defaultOpenCLClimateBatch
	}

	openCLLoaderMu.Lock()
	if err := configureOpenCLICD(cfg.ICDLibraryPath); err != nil {
		openCLLoaderMu.Unlock()
		return nil, fmt.Errorf("%w: %v", ErrAcceleratorUnavailable, err)
	}
	var path *C.char
	if cfg.LibraryPath != "" {
		path = C.CString(cfg.LibraryPath)
		defer C.free(unsafe.Pointer(path))
	}
	loaded := C.vg_cl_load(path)
	openCLLoaderMu.Unlock()
	if loaded != 0 {
		return nil, fmt.Errorf("%w: %s", ErrAcceleratorUnavailable, C.GoString(C.vg_cl_last_error()))
	}

	platformIndex, deviceIndex := cfg.PlatformIndex, cfg.DeviceIndex
	if platformIndex < 0 {
		platformIndex = 0
	}
	if deviceIndex < 0 {
		deviceIndex = 0
	}
	var platform C.cl_platform_id
	var device C.cl_device_id
	if status := C.vg_cl_pick_gpu(C.int(platformIndex), C.int(deviceIndex), &platform, &device); status != C.CL_SUCCESS {
		return nil, fmt.Errorf("%w: %v", ErrAcceleratorUnavailable, openCLError("select GPU", status))
	}
	name := openCLDeviceString(device, C.CL_DEVICE_NAME)
	vendor := openCLDeviceString(device, C.CL_DEVICE_VENDOR)
	extensions := openCLDeviceString(device, C.CL_DEVICE_EXTENSIONS)
	var status C.cl_int
	doubleConfig := C.vg_cl_device_ulong(device, C.CL_DEVICE_DOUBLE_FP_CONFIG, &status)
	if status != C.CL_SUCCESS || (doubleConfig == 0 && !strings.Contains(extensions, "cl_khr_fp64")) {
		return nil, fmt.Errorf("%w: %s does not support float64", ErrAcceleratorUnavailable, name)
	}

	a := &openCLAccelerator{
		device:        device,
		info:          AcceleratorInfo{Backend: "opencl", Device: name, Vendor: vendor},
		noises:        noises,
		climateBatch:  batch,
		columnScratch: make([]int32, batch*2),
		biomeScratch:  make([]int32, batch*2),
		columnStarts:  make([]int, batch+1),
		climateData:   make([]float64, batch*6),
	}
	a.context = C.vg_cl_create_context(device, &status)
	if status != C.CL_SUCCESS {
		return nil, openCLError("create context", status)
	}
	a.queue = C.vg_cl_create_queue(a.context, device, &status)
	if status != C.CL_SUCCESS {
		a.Close()
		return nil, openCLError("create command queue", status)
	}

	splineSource, err := generateOpenCLTerrainSpline(OverworldGraph.Nodes[40].Spline)
	if err != nil {
		a.Close()
		return nil, err
	}
	source := strings.Replace(openCLOverworldBaseSource, "/*__SPLINE_FUNCTIONS__*/", splineSource, 1)
	cSource := C.CString(source)
	a.program = C.vg_cl_create_program(a.context, cSource, C.size_t(len(source)), &status)
	C.free(unsafe.Pointer(cSource))
	if status != C.CL_SUCCESS {
		a.Close()
		return nil, openCLError("create program", status)
	}
	options := C.CString("-cl-std=CL1.2")
	status = C.vg_cl_build_program(a.program, device, options)
	C.free(unsafe.Pointer(options))
	if status != C.CL_SUCCESS {
		log := C.vg_cl_build_log(a.program, device)
		message := ""
		if log != nil {
			message = strings.TrimSpace(C.GoString(log))
			C.free(unsafe.Pointer(log))
		}
		a.Close()
		return nil, fmt.Errorf("OpenCL build program: %s: %s", openCLErrorText(status), message)
	}
	if a.density, err = openCLCreateKernel(a.program, "vg_final_density_kernel"); err != nil {
		a.Close()
		return nil, err
	}
	if a.climate, err = openCLCreateKernel(a.program, "vg_climate_kernel"); err != nil {
		a.Close()
		return nil, err
	}
	if a.biome, err = openCLCreateKernel(a.program, "vg_biome_kernel"); err != nil {
		a.Close()
		return nil, err
	}

	packed := packOpenCLNoise(noises)
	noiseMeta, err := a.createStaticBuffer(packed.noiseMeta)
	if err != nil {
		a.Close()
		return nil, err
	}
	noiseAmplitudes, err := a.createStaticBuffer(packed.noiseAmplitudes)
	if err != nil {
		a.Close()
		return nil, err
	}
	perlinParams, err := a.createStaticBuffer(packed.perlinParams)
	if err != nil {
		a.Close()
		return nil, err
	}
	cachedH2, err := a.createStaticBuffer(packed.cachedH2)
	if err != nil {
		a.Close()
		return nil, err
	}
	permutations, err := a.createStaticBuffer(packed.permutations)
	if err != nil {
		a.Close()
		return nil, err
	}
	blendMeta, err := a.createStaticBuffer(packed.blendMeta)
	if err != nil {
		a.Close()
		return nil, err
	}
	blendParams, err := a.createStaticBuffer(packed.blendParams)
	if err != nil {
		a.Close()
		return nil, err
	}
	tree := packOpenCLClimateTree(overworldClimateRTree())
	if tree.maxDepth > 16 {
		a.Close()
		return nil, fmt.Errorf("OpenCL climate tree depth %d exceeds kernel stack", tree.maxDepth)
	}
	treeMeta, err := a.createStaticBuffer(tree.meta)
	if err != nil {
		a.Close()
		return nil, err
	}
	treeSpaces, err := a.createStaticBuffer(tree.spaces)
	if err != nil {
		a.Close()
		return nil, err
	}
	treeBiomes, err := a.createStaticBuffer(tree.biomes)
	if err != nil {
		a.Close()
		return nil, err
	}

	for i, memory := range []C.cl_mem{noiseMeta, noiseAmplitudes, perlinParams, cachedH2, permutations, blendMeta, blendParams} {
		if status = C.vg_cl_set_mem(a.density, C.cl_uint(i), memory); status != C.CL_SUCCESS {
			a.Close()
			return nil, openCLError("set density static argument", status)
		}
	}
	for i, memory := range []C.cl_mem{noiseMeta, noiseAmplitudes, perlinParams, cachedH2, permutations} {
		if status = C.vg_cl_set_mem(a.climate, C.cl_uint(i), memory); status != C.CL_SUCCESS {
			a.Close()
			return nil, openCLError("set climate static argument", status)
		}
	}
	for i, memory := range []C.cl_mem{treeMeta, treeSpaces, treeBiomes} {
		if status = C.vg_cl_set_mem(a.biome, C.cl_uint(i), memory); status != C.CL_SUCCESS {
			a.Close()
			return nil, openCLError("set biome static argument", status)
		}
	}

	if a.chunkMeta, err = a.createBuffer(C.CL_MEM_READ_ONLY, 4*4); err != nil {
		a.Close()
		return nil, err
	}
	if a.densityIn, err = a.createBuffer(C.CL_MEM_READ_ONLY, openCLDensityInputs*8); err != nil {
		a.Close()
		return nil, err
	}
	if a.densityOut, err = a.createBuffer(C.CL_MEM_WRITE_ONLY, openCLMaxDensityCorners*8); err != nil {
		a.Close()
		return nil, err
	}
	if a.climateIn, err = a.createBuffer(C.CL_MEM_READ_ONLY, batch*2*4); err != nil {
		a.Close()
		return nil, err
	}
	if a.climateOut, err = a.createBuffer(C.CL_MEM_READ_WRITE, batch*6*8); err != nil {
		a.Close()
		return nil, err
	}
	if a.biomeIn, err = a.createBuffer(C.CL_MEM_READ_ONLY, batch*2*4); err != nil {
		a.Close()
		return nil, err
	}
	if a.biomeOut, err = a.createBuffer(C.CL_MEM_WRITE_ONLY, batch); err != nil {
		a.Close()
		return nil, err
	}

	for i, memory := range []C.cl_mem{a.chunkMeta, a.densityIn, a.densityOut} {
		if status = C.vg_cl_set_mem(a.density, C.cl_uint(7+i), memory); status != C.CL_SUCCESS {
			a.Close()
			return nil, openCLError("set density dynamic argument", status)
		}
	}
	for i, memory := range []C.cl_mem{a.climateIn, a.climateOut} {
		if status = C.vg_cl_set_mem(a.climate, C.cl_uint(5+i), memory); status != C.CL_SUCCESS {
			a.Close()
			return nil, openCLError("set climate dynamic argument", status)
		}
	}
	for i, memory := range []C.cl_mem{a.biomeIn, a.climateOut, a.biomeOut} {
		if status = C.vg_cl_set_mem(a.biome, C.cl_uint(3+i), memory); status != C.CL_SUCCESS {
			a.Close()
			return nil, openCLError("set biome dynamic argument", status)
		}
	}
	return a, nil
}

func (a *openCLAccelerator) Name() string { return "opencl:" + a.info.Device }

func (a *openCLAccelerator) AcceleratorInfo() AcceleratorInfo { return a.info }

func (a *openCLAccelerator) FinalDensity(chunkX, chunkZ, minY, maxY int, flat *FlatCacheGrid) (*FinalDensityChunk, error) {
	if flat == nil {
		return nil, fmt.Errorf("OpenCL final density: nil flat cache")
	}
	cellCountY := (maxY - minY + 1) / 8
	if cellCountY < 0 || cellCountY > 48 {
		return nil, fmt.Errorf("OpenCL final density: unsupported height %d..%d", minY, maxY)
	}
	cornerYCount := cellCountY + 1
	pointCount := openCLDensityColumns * cornerYCount
	var inputs [openCLDensityInputs]float64
	for cornerX := 0; cornerX <= 4; cornerX++ {
		worldX := chunkX*16 + cornerX*4
		for cornerZ := 0; cornerZ <= 4; cornerZ++ {
			worldZ := chunkZ*16 + cornerZ*4
			column := OverworldGraph.NewColumnContext(worldX, worldZ, a.noises, flat)
			base := (cornerX*5 + cornerZ) * 3
			inputs[base] = flat.Lookup(6, worldX, worldZ)
			inputs[base+1] = column.values[5]
			inputs[base+2] = column.values[6]
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil, fmt.Errorf("OpenCL accelerator is closed")
	}
	meta := [4]int32{int32(chunkX * 16), int32(chunkZ * 16), int32(minY), int32(cornerYCount)}
	if err := a.writeInt32(a.chunkMeta, meta[:]); err != nil {
		return nil, err
	}
	if err := a.writeFloat64(a.densityIn, inputs[:]); err != nil {
		return nil, err
	}
	if status := C.vg_cl_run_1d(a.queue, a.density, C.size_t(pointCount)); status != C.CL_SUCCESS {
		return nil, openCLError("run final-density kernel", status)
	}
	var output [openCLMaxDensityCorners]float64
	if err := a.readFloat64(a.densityOut, output[:pointCount]); err != nil {
		return nil, err
	}
	chunk := &FinalDensityChunk{
		minY: minY, baseX: chunkX * 16, baseZ: chunkZ * 16, cellCountY: cellCountY,
	}
	for cornerX := 0; cornerX <= 4; cornerX++ {
		for cornerZ := 0; cornerZ <= 4; cornerZ++ {
			base := (cornerX*5 + cornerZ) * cornerYCount
			copy(chunk.corners[cornerX][cornerZ][:cornerYCount], output[base:base+cornerYCount])
		}
	}
	return chunk, nil
}

func (a *openCLAccelerator) SampleClimate(points []FunctionContext, dst [][6]int64) error {
	if len(dst) < len(points) {
		return fmt.Errorf("OpenCL climate destination has length %d, need %d", len(dst), len(points))
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return fmt.Errorf("OpenCL accelerator is closed")
	}
	for start := 0; start < len(points); start += a.climateBatch {
		end := min(start+a.climateBatch, len(points))
		columnCount := a.packClimateColumns(points, start, end, false)
		if err := a.writeInt32(a.climateIn, a.columnScratch[:columnCount*2]); err != nil {
			return err
		}
		if status := C.vg_cl_run_1d(a.queue, a.climate, C.size_t(columnCount)); status != C.CL_SUCCESS {
			return openCLError("run climate kernel", status)
		}
		if err := a.readFloat64(a.climateOut, a.climateData[:columnCount*6]); err != nil {
			return err
		}
		for column := 0; column < columnCount; column++ {
			values := a.climateData[column*6 : column*6+6]
			for pointIndex := start + a.columnStarts[column]; pointIndex < start+a.columnStarts[column+1]; pointIndex++ {
				depth := yClampedGradient(points[pointIndex].BlockY, -64, 320, 1.5, -1.5) + values[4]
				dst[pointIndex] = [6]int64{
					quantizeOpenCLClimate(values[0]),
					quantizeOpenCLClimate(values[1]),
					quantizeOpenCLClimate(values[2]),
					quantizeOpenCLClimate(values[3]),
					quantizeOpenCLClimate(depth),
					quantizeOpenCLClimate(values[5]),
				}
			}
		}
	}
	return nil
}

func (a *openCLAccelerator) SampleBiomes(points []FunctionContext, dst []Biome) error {
	if len(dst) < len(points) {
		return fmt.Errorf("OpenCL biome destination has length %d, need %d", len(dst), len(points))
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return fmt.Errorf("OpenCL accelerator is closed")
	}
	for start := 0; start < len(points); start += a.climateBatch {
		end := min(start+a.climateBatch, len(points))
		columnCount := a.packClimateColumns(points, start, end, true)
		if err := a.writeInt32(a.climateIn, a.columnScratch[:columnCount*2]); err != nil {
			return err
		}
		if err := a.writeInt32(a.biomeIn, a.biomeScratch[:(end-start)*2]); err != nil {
			return err
		}
		if status := C.vg_cl_run_1d(a.queue, a.climate, C.size_t(columnCount)); status != C.CL_SUCCESS {
			return openCLError("run climate kernel", status)
		}
		if status := C.vg_cl_run_1d(a.queue, a.biome, C.size_t(end-start)); status != C.CL_SUCCESS {
			return openCLError("run biome kernel", status)
		}
		if err := a.readBiomes(a.biomeOut, dst[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (a *openCLAccelerator) packClimateColumns(points []FunctionContext, start, end int, includeBiomePoints bool) int {
	columnCount := 0
	for pointIndex := start; pointIndex < end; {
		point := points[pointIndex]
		base := columnCount * 2
		a.columnScratch[base] = int32(point.BlockX)
		a.columnScratch[base+1] = int32(point.BlockZ)
		a.columnStarts[columnCount] = pointIndex - start
		columnCount++
		for ; pointIndex < end && points[pointIndex].BlockX == point.BlockX && points[pointIndex].BlockZ == point.BlockZ; pointIndex++ {
			if includeBiomePoints {
				pointBase := (pointIndex - start) * 2
				a.biomeScratch[pointBase] = int32(points[pointIndex].BlockY)
				a.biomeScratch[pointBase+1] = int32(columnCount - 1)
			}
		}
	}
	a.columnStarts[columnCount] = end - start
	return columnCount
}

func quantizeOpenCLClimate(value float64) int64 {
	return int64(float32(value) * float32(10000.0))
}

func (a *openCLAccelerator) Close() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	for _, memory := range []C.cl_mem{a.biomeOut, a.biomeIn, a.climateOut, a.climateIn, a.densityOut, a.densityIn, a.chunkMeta} {
		C.vg_cl_release_mem(memory)
	}
	for i := len(a.staticMem) - 1; i >= 0; i-- {
		C.vg_cl_release_mem(a.staticMem[i])
	}
	C.vg_cl_release_kernel(a.biome)
	C.vg_cl_release_kernel(a.climate)
	C.vg_cl_release_kernel(a.density)
	C.vg_cl_release_program(a.program)
	C.vg_cl_release_queue(a.queue)
	C.vg_cl_release_context(a.context)
	return nil
}

func (a *openCLAccelerator) createBuffer(flags C.cl_mem_flags, bytes int) (C.cl_mem, error) {
	var status C.cl_int
	memory := C.vg_cl_create_buffer(a.context, flags, C.size_t(bytes), nil, &status)
	if status != C.CL_SUCCESS {
		return nil, openCLError("create buffer", status)
	}
	return memory, nil
}

func (a *openCLAccelerator) createStaticBuffer(data any) (C.cl_mem, error) {
	ptr, bytes, err := openCLSlicePointer(data)
	if err != nil {
		return nil, err
	}
	var status C.cl_int
	memory := C.vg_cl_create_buffer(a.context, C.CL_MEM_READ_ONLY|C.CL_MEM_COPY_HOST_PTR, C.size_t(bytes), ptr, &status)
	if status != C.CL_SUCCESS {
		return nil, openCLError("create static buffer", status)
	}
	a.staticMem = append(a.staticMem, memory)
	return memory, nil
}

func (a *openCLAccelerator) writeInt32(memory C.cl_mem, values []int32) error {
	if len(values) == 0 {
		return nil
	}
	bytes := len(values) * 4
	if status := C.vg_cl_write(a.queue, memory, C.size_t(bytes), unsafe.Pointer(&values[0])); status != C.CL_SUCCESS {
		return openCLError("write buffer", status)
	}
	return nil
}

func (a *openCLAccelerator) writeFloat64(memory C.cl_mem, values []float64) error {
	if len(values) == 0 {
		return nil
	}
	bytes := len(values) * 8
	if status := C.vg_cl_write(a.queue, memory, C.size_t(bytes), unsafe.Pointer(&values[0])); status != C.CL_SUCCESS {
		return openCLError("write buffer", status)
	}
	return nil
}

func (a *openCLAccelerator) readFloat64(memory C.cl_mem, values []float64) error {
	if len(values) == 0 {
		return nil
	}
	bytes := len(values) * 8
	if status := C.vg_cl_read(a.queue, memory, C.size_t(bytes), unsafe.Pointer(&values[0])); status != C.CL_SUCCESS {
		return openCLError("read buffer", status)
	}
	return nil
}

func (a *openCLAccelerator) readBiomes(memory C.cl_mem, values []Biome) error {
	if len(values) == 0 {
		return nil
	}
	if status := C.vg_cl_read(a.queue, memory, C.size_t(len(values)), unsafe.Pointer(&values[0])); status != C.CL_SUCCESS {
		return openCLError("read buffer", status)
	}
	return nil
}

type openCLClimateTreeData struct {
	meta     []int32
	spaces   []int64
	biomes   []uint8
	maxDepth int
}

func packOpenCLClimateTree(tree *climateRTree) openCLClimateTreeData {
	type queuedNode struct {
		node  *climateRTreeNode
		depth int
	}
	queue := []queuedNode{{node: tree.root, depth: 1}}
	data := openCLClimateTreeData{}
	for index := 0; index < len(queue); index++ {
		entry := queue[index]
		if entry.depth > data.maxDepth {
			data.maxDepth = entry.depth
		}
		firstChild := int32(0)
		if len(entry.node.children) > 0 {
			firstChild = int32(len(queue))
			for _, child := range entry.node.children {
				queue = append(queue, queuedNode{node: child, depth: entry.depth + 1})
			}
		}
		data.meta = append(data.meta, firstChild, int32(len(entry.node.children)))
		for _, dimension := range entry.node.space {
			data.spaces = append(data.spaces, dimension.min, dimension.max)
		}
		data.biomes = append(data.biomes, uint8(entry.node.biome))
	}
	return data
}

type openCLNoiseData struct {
	noiseMeta       []int32
	noiseAmplitudes []float64
	perlinParams    []float64
	cachedH2        []int32
	permutations    []int32
	blendMeta       []int32
	blendParams     []float64
}

func packOpenCLNoise(registry *NoiseRegistry) openCLNoiseData {
	data := openCLNoiseData{
		noiseMeta:       make([]int32, len(registry.noises)*4),
		noiseAmplitudes: make([]float64, len(registry.noises)),
	}
	appendPerlin := func(perlin PerlinNoise) {
		data.perlinParams = append(data.perlinParams,
			perlin.a, perlin.b, perlin.c, perlin.amplitude,
			perlin.valueFactor, perlin.lacunarity, perlin.d2, perlin.t2,
		)
		data.cachedH2 = append(data.cachedH2, perlin.h2)
		data.permutations = append(data.permutations, perlin.d[:]...)
	}
	appendOctaves := func(octaves OctaveNoise) (int32, int32) {
		offset := int32(len(data.cachedH2))
		for _, perlin := range octaves.octaves {
			appendPerlin(perlin)
		}
		return offset, int32(len(octaves.octaves))
	}
	for i, noise := range registry.noises {
		aOffset, aCount := appendOctaves(noise.octA)
		bOffset, bCount := appendOctaves(noise.octB)
		copy(data.noiseMeta[i*4:i*4+4], []int32{aOffset, aCount, bOffset, bCount})
		data.noiseAmplitudes[i] = noise.amplitude
	}
	minOffset, minCount := appendOctaves(registry.blendedNoise.minLimit)
	maxOffset, maxCount := appendOctaves(registry.blendedNoise.maxLimit)
	mainOffset, mainCount := appendOctaves(registry.blendedNoise.main)
	data.blendMeta = []int32{minOffset, minCount, maxOffset, maxCount, mainOffset, mainCount}
	data.blendParams = []float64{
		registry.blendedNoise.xzMultiplier,
		registry.blendedNoise.yMultiplier,
		registry.blendedNoise.xzFactor,
		registry.blendedNoise.yFactor,
		registry.blendedNoise.limitSmearScale,
		registry.blendedNoise.mainSmearScale,
	}
	return data
}

func openCLSlicePointer(data any) (unsafe.Pointer, int, error) {
	switch values := data.(type) {
	case []int32:
		if len(values) == 0 {
			return nil, 0, fmt.Errorf("empty OpenCL int32 buffer")
		}
		return unsafe.Pointer(&values[0]), len(values) * 4, nil
	case []float64:
		if len(values) == 0 {
			return nil, 0, fmt.Errorf("empty OpenCL float64 buffer")
		}
		return unsafe.Pointer(&values[0]), len(values) * 8, nil
	case []int64:
		if len(values) == 0 {
			return nil, 0, fmt.Errorf("empty OpenCL int64 buffer")
		}
		return unsafe.Pointer(&values[0]), len(values) * 8, nil
	case []uint8:
		if len(values) == 0 {
			return nil, 0, fmt.Errorf("empty OpenCL uint8 buffer")
		}
		return unsafe.Pointer(&values[0]), len(values), nil
	default:
		return nil, 0, fmt.Errorf("unsupported OpenCL buffer type %T", data)
	}
}

func openCLCreateKernel(program C.cl_program, name string) (C.cl_kernel, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	var status C.cl_int
	kernel := C.vg_cl_create_kernel(program, cName, &status)
	if status != C.CL_SUCCESS {
		return nil, openCLError("create kernel "+name, status)
	}
	return kernel, nil
}

func openCLDeviceString(device C.cl_device_id, param C.cl_device_info) string {
	value := C.vg_cl_device_string(device, param)
	if value == nil {
		return "unknown"
	}
	defer C.free(unsafe.Pointer(value))
	return C.GoString(value)
}

func openCLError(operation string, status C.cl_int) error {
	return fmt.Errorf("OpenCL %s: %s (%d)", operation, openCLErrorText(status), int32(status))
}

func openCLErrorText(status C.cl_int) string {
	switch int32(status) {
	case 0:
		return "success"
	case -1:
		return "device not found"
	case -5:
		return "out of resources"
	case -6:
		return "out of host memory"
	case -11:
		return "program build failure"
	case -30:
		return "invalid value"
	case -32:
		return "invalid platform"
	case -33:
		return "invalid device"
	case -34:
		return "invalid context"
	case -36:
		return "invalid command queue"
	case -38:
		return "invalid memory object"
	case -48:
		return "invalid kernel"
	case -49:
		return "invalid argument index"
	case -50:
		return "invalid argument value"
	case -51:
		return "invalid argument size"
	case -52:
		return "invalid kernel arguments"
	case -54:
		return "invalid work-group size"
	case -1001:
		return "platform not found"
	default:
		return "error"
	}
}

type openCLSplineGenerator struct {
	ids       map[*Spline]int
	functions []string
}

func generateOpenCLTerrainSpline(root *Spline) (string, error) {
	if root == nil {
		return "", fmt.Errorf("OpenCL terrain offset spline is nil")
	}
	g := &openCLSplineGenerator{ids: make(map[*Spline]int)}
	rootID, err := g.add(root)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, function := range g.functions {
		out.WriteString(function)
	}
	fmt.Fprintf(&out, "\ninline float vg_terrain_offset(double continental, double erosion, double ridges, double peaks_valleys) { return vg_spline_%d(continental, erosion, ridges, peaks_valleys); }\n", rootID)
	return out.String(), nil
}

func (g *openCLSplineGenerator) add(spline *Spline) (int, error) {
	if id, ok := g.ids[spline]; ok {
		return id, nil
	}
	id := len(g.ids)
	g.ids[spline] = id
	for _, point := range spline.Points {
		if point.Value.Nested != nil {
			if _, err := g.add(point.Value.Nested); err != nil {
				return 0, err
			}
		}
	}
	coord, err := openCLSplineArg(spline.Coordinate)
	if err != nil {
		return 0, err
	}
	if len(spline.Points) == 0 {
		return 0, fmt.Errorf("OpenCL spline %d has no points", id)
	}
	var body strings.Builder
	fmt.Fprintf(&body, "\ninline float vg_spline_%d(double continental, double erosion, double ridges, double peaks_valleys) {\n", id)
	fmt.Fprintf(&body, "    float coord = (float)(%s);\n", coord)
	first := spline.Points[0]
	firstValue := g.value(first.Value)
	fmt.Fprintf(&body, "    if (coord < %s) return %s + %s * (coord - %s);\n", openCLFloat(first.Location), firstValue, openCLFloat(first.Derivative), openCLFloat(first.Location))
	for i := 0; i < len(spline.Points)-1; i++ {
		left, right := spline.Points[i], spline.Points[i+1]
		leftValue, rightValue := g.value(left.Value), g.value(right.Value)
		fmt.Fprintf(&body, "    if (coord < %s) {\n", openCLFloat(right.Location))
		fmt.Fprintf(&body, "        float f3 = (coord - %s) / (%s - %s);\n", openCLFloat(left.Location), openCLFloat(right.Location), openCLFloat(left.Location))
		fmt.Fprintf(&body, "        float f6 = %s;\n", leftValue)
		fmt.Fprintf(&body, "        float f7 = %s;\n", rightValue)
		fmt.Fprintf(&body, "        float f8 = %s * (%s - %s) - (f7 - f6);\n", openCLFloat(left.Derivative), openCLFloat(right.Location), openCLFloat(left.Location))
		fmt.Fprintf(&body, "        float f9 = -%s * (%s - %s) + (f7 - f6);\n", openCLFloat(right.Derivative), openCLFloat(right.Location), openCLFloat(left.Location))
		body.WriteString("        return vg_lerpf(f3, f6, f7) + f3 * (1.0f - f3) * vg_lerpf(f3, f8, f9);\n    }\n")
	}
	last := spline.Points[len(spline.Points)-1]
	fmt.Fprintf(&body, "    return %s + %s * (coord - %s);\n}\n", g.value(last.Value), openCLFloat(last.Derivative), openCLFloat(last.Location))
	g.functions = append(g.functions, body.String())
	return id, nil
}

func (g *openCLSplineGenerator) value(value SplineValue) string {
	if value.Nested == nil {
		return openCLFloat(value.Const)
	}
	id := g.ids[value.Nested]
	return fmt.Sprintf("vg_spline_%d(continental, erosion, ridges, peaks_valleys)", id)
}

func openCLSplineArg(arg ArgRef) (string, error) {
	if arg.Node < 0 {
		return strconv.FormatFloat(arg.Const, 'g', -1, 64), nil
	}
	switch arg.Node {
	case 27:
		return "continental", nil
	case 29:
		return "erosion", nil
	case 34:
		return "ridges", nil
	case 39:
		return "peaks_valleys", nil
	default:
		return "", fmt.Errorf("OpenCL terrain spline has unsupported coordinate node %d", arg.Node)
	}
}

func openCLFloat(value float64) string {
	formatted := strconv.FormatFloat(float64(float32(value)), 'g', -1, 32)
	if !strings.ContainsAny(formatted, ".eE") {
		formatted += ".0"
	}
	return formatted + "f"
}
