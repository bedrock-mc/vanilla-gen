#pragma OPENCL EXTENSION cl_khr_fp64 : enable
#pragma OPENCL FP_CONTRACT OFF

// The buffers use a structure-of-arrays layout. This avoids host/device ABI
// padding differences and gives adjacent work items coalesced reads.
// perlin_params: a,b,c,amplitude,valueFactor,lacunarity,d2,t2

__constant double vg_gradients[48] = {
     1.0,  1.0,  0.0,  -1.0,  1.0,  0.0,
     1.0, -1.0,  0.0,  -1.0, -1.0,  0.0,
     1.0,  0.0,  1.0,  -1.0,  0.0,  1.0,
     1.0,  0.0, -1.0,  -1.0,  0.0, -1.0,
     0.0,  1.0,  1.0,   0.0, -1.0,  1.0,
     0.0,  1.0, -1.0,   0.0, -1.0, -1.0,
     1.0,  1.0,  0.0,   0.0, -1.0,  1.0,
    -1.0,  1.0,  0.0,   0.0, -1.0, -1.0
};

inline double vg_lerp(double t, double a, double b) {
    return a + t * (b - a);
}

inline float vg_lerpf(float t, float a, float b) {
    return a + t * (b - a);
}

inline double vg_min(double a, double b) {
    return a < b ? a : b;
}

inline double vg_max(double a, double b) {
    return a > b ? a : b;
}

inline double vg_clamp(double v, double lo, double hi) {
    if (v < lo) return lo;
    if (v > hi) return hi;
    return v;
}

inline double vg_smoothstep(double d) {
    return d * d * d * (d * (d * 6.0 - 15.0) + 10.0);
}

inline double vg_wrap(double v) {
    const double r = 33554432.0;
    return v - floor(v / r + 0.5) * r;
}

inline double vg_gradient(int index, double x, double y, double z) {
    int base = (index & 15) * 3;
    return vg_gradients[base] * x + vg_gradients[base + 1] * y + vg_gradients[base + 2] * z;
}

inline double vg_perlin(
    int perlin,
    double x,
    double y,
    double z,
    __global const double *params,
    __global const int *cached_h2,
    __global const int *permutations
) {
    int po = perlin * 8;
    double d2;
    double t2;
    int h2;
    if (y == 0.0) {
        d2 = params[po + 6];
        h2 = cached_h2[perlin];
        t2 = params[po + 7];
    } else {
        double yv = y + params[po + 1];
        double i2 = floor(yv);
        d2 = yv - i2;
        h2 = ((int)i2) & 255;
        t2 = vg_smoothstep(d2);
    }

    double d1 = x + params[po];
    double d3 = z + params[po + 2];
    double i1 = floor(d1);
    double i3 = floor(d3);
    d1 -= i1;
    d3 -= i3;

    int h1 = ((int)i1) & 255;
    int h3 = ((int)i3) & 255;
    double t1 = vg_smoothstep(d1);
    double t3 = vg_smoothstep(d3);
    int perm = perlin * 257;

    int a1 = (permutations[perm + h1] + h2) & 255;
    int b1 = (permutations[perm + ((h1 + 1) & 255)] + h2) & 255;
    int a2 = (permutations[perm + a1] + h3) & 255;
    int a3 = (permutations[perm + ((a1 + 1) & 255)] + h3) & 255;
    int b2 = (permutations[perm + b1] + h3) & 255;
    int b3 = (permutations[perm + ((b1 + 1) & 255)] + h3) & 255;

    double l1 = vg_gradient(permutations[perm + a2] & 15, d1, d2, d3);
    double l2 = vg_gradient(permutations[perm + b2] & 15, d1 - 1.0, d2, d3);
    double l3 = vg_gradient(permutations[perm + a3] & 15, d1, d2 - 1.0, d3);
    double l4 = vg_gradient(permutations[perm + b3] & 15, d1 - 1.0, d2 - 1.0, d3);
    double l5 = vg_gradient(permutations[perm + ((a2 + 1) & 255)] & 15, d1, d2, d3 - 1.0);
    double l6 = vg_gradient(permutations[perm + ((b2 + 1) & 255)] & 15, d1 - 1.0, d2, d3 - 1.0);
    double l7 = vg_gradient(permutations[perm + ((a3 + 1) & 255)] & 15, d1, d2 - 1.0, d3 - 1.0);
    double l8 = vg_gradient(permutations[perm + ((b3 + 1) & 255)] & 15, d1 - 1.0, d2 - 1.0, d3 - 1.0);

    l1 = vg_lerp(t1, l1, l2);
    l3 = vg_lerp(t1, l3, l4);
    l5 = vg_lerp(t1, l5, l6);
    l7 = vg_lerp(t1, l7, l8);
    l1 = vg_lerp(t2, l1, l3);
    l5 = vg_lerp(t2, l5, l7);
    return vg_lerp(t3, l1, l5);
}

inline double vg_perlin_smeared(
    int perlin,
    double x,
    double y,
    double z,
    double y_scale,
    double y_orig,
    __global const double *params,
    __global const int *permutations
) {
    int po = perlin * 8;
    double d1 = x + params[po];
    double d2_raw = y + params[po + 1];
    double d3 = z + params[po + 2];
    double i1 = floor(d1);
    double i2 = floor(d2_raw);
    double i3 = floor(d3);
    d1 -= i1;
    double d2 = d2_raw - i2;
    d3 -= i3;

    double s = 0.0;
    if (y_scale != 0.0) {
        double r = d2;
        if (y_orig >= 0.0 && y_orig < d2) r = y_orig;
        s = floor(r / y_scale + 1.0e-7) * y_scale;
    }
    double d2s = d2 - s;
    int h1 = ((int)i1) & 255;
    int h2 = ((int)i2) & 255;
    int h3 = ((int)i3) & 255;
    double t1 = vg_smoothstep(d1);
    double t2 = vg_smoothstep(d2);
    double t3 = vg_smoothstep(d3);
    int perm = perlin * 257;

    int a1 = (permutations[perm + h1] + h2) & 255;
    int b1 = (permutations[perm + ((h1 + 1) & 255)] + h2) & 255;
    int a2 = (permutations[perm + a1] + h3) & 255;
    int a3 = (permutations[perm + ((a1 + 1) & 255)] + h3) & 255;
    int b2 = (permutations[perm + b1] + h3) & 255;
    int b3 = (permutations[perm + ((b1 + 1) & 255)] + h3) & 255;

    double l1 = vg_gradient(permutations[perm + a2] & 15, d1, d2s, d3);
    double l2 = vg_gradient(permutations[perm + b2] & 15, d1 - 1.0, d2s, d3);
    double l3 = vg_gradient(permutations[perm + a3] & 15, d1, d2s - 1.0, d3);
    double l4 = vg_gradient(permutations[perm + b3] & 15, d1 - 1.0, d2s - 1.0, d3);
    double l5 = vg_gradient(permutations[perm + ((a2 + 1) & 255)] & 15, d1, d2s, d3 - 1.0);
    double l6 = vg_gradient(permutations[perm + ((b2 + 1) & 255)] & 15, d1 - 1.0, d2s, d3 - 1.0);
    double l7 = vg_gradient(permutations[perm + ((a3 + 1) & 255)] & 15, d1, d2s - 1.0, d3 - 1.0);
    double l8 = vg_gradient(permutations[perm + ((b3 + 1) & 255)] & 15, d1 - 1.0, d2s - 1.0, d3 - 1.0);

    l1 = vg_lerp(t1, l1, l2);
    l3 = vg_lerp(t1, l3, l4);
    l5 = vg_lerp(t1, l5, l6);
    l7 = vg_lerp(t1, l7, l8);
    l1 = vg_lerp(t2, l1, l3);
    l5 = vg_lerp(t2, l5, l7);
    return vg_lerp(t3, l1, l5);
}

inline double vg_octaves(
    int offset,
    int count,
    double x,
    double y,
    double z,
    __global const double *params,
    __global const int *cached_h2,
    __global const int *permutations
) {
    double value = 0.0;
    for (int i = 0; i < count; i++) {
        int p = offset + i;
        int po = p * 8;
        double frequency = params[po + 5];
        double sample = vg_perlin(p, vg_wrap(x * frequency), vg_wrap(y * frequency), vg_wrap(z * frequency), params, cached_h2, permutations);
        value += params[po + 3] * sample * params[po + 4];
    }
    return value;
}

inline double vg_noise(
    int noise,
    double x,
    double y,
    double z,
    __global const int *noise_meta,
    __global const double *noise_amplitudes,
    __global const double *params,
    __global const int *cached_h2,
    __global const int *permutations
) {
    int no = noise * 4;
    double a = vg_octaves(noise_meta[no], noise_meta[no + 1], x, y, z, params, cached_h2, permutations);
    const double input_factor = 1.0181268882175227;
    double b = vg_octaves(noise_meta[no + 2], noise_meta[no + 3], x * input_factor, y * input_factor, z * input_factor, params, cached_h2, permutations);
    return (a + b) * noise_amplitudes[noise];
}

inline double vg_blended(
    double x,
    double y,
    double z,
    __global const int *blend_meta,
    __global const double *blend_params,
    __global const double *params,
    __global const int *permutations
) {
    double dx = x * blend_params[0];
    double dy = y * blend_params[1];
    double dz = z * blend_params[0];
    double gx = dx / blend_params[2];
    double gy = dy / blend_params[3];
    double gz = dz / blend_params[2];

    double n = 0.0;
    double o = 1.0;
    for (int i = 0; i < 8; i++) {
        if (i < blend_meta[5]) {
            n += vg_perlin_smeared(blend_meta[4] + i, vg_wrap(gx * o), vg_wrap(gy * o), vg_wrap(gz * o), blend_params[5] * o, gy * o, params, permutations) / o;
        }
        o /= 2.0;
    }

    double q = (n / 10.0 + 1.0) / 2.0;
    int max_only = q >= 1.0;
    int min_only = q <= 0.0;
    double l = 0.0;
    double m = 0.0;
    o = 1.0;
    for (int i = 0; i < 16; i++) {
        if (!max_only && i < blend_meta[1]) {
            l += vg_perlin_smeared(blend_meta[0] + i, vg_wrap(dx * o), vg_wrap(dy * o), vg_wrap(dz * o), blend_params[4] * o, dy * o, params, permutations) / o;
        }
        if (!min_only && i < blend_meta[3]) {
            m += vg_perlin_smeared(blend_meta[2] + i, vg_wrap(dx * o), vg_wrap(dy * o), vg_wrap(dz * o), blend_params[4] * o, dy * o, params, permutations) / o;
        }
        o /= 2.0;
    }
    double t = vg_clamp(q, 0.0, 1.0);
    return vg_lerp(t, l / 512.0, m / 512.0) / 128.0;
}

inline double vg_y_gradient(int y, int from_y, int to_y, double from_value, double to_value) {
    if (y <= from_y) return from_value;
    if (y >= to_y) return to_value;
    double t = (double)(y - from_y) / (double)(to_y - from_y);
    return from_value + t * (to_value - from_value);
}

inline double vg_rarity1(double v) {
    if (v < -0.5) return 0.75;
    if (v < 0.0) return 1.0;
    if (v < 0.5) return 1.5;
    return 2.0;
}

inline double vg_rarity2(double v) {
    if (v < -0.75) return 0.5;
    if (v < -0.5) return 0.75;
    if (v < 0.5) return 1.0;
    if (v < 0.75) return 2.0;
    return 3.0;
}

inline double vg_final_density(
    int x,
    int y,
    int z,
    double flat6,
    double column5,
    double column6,
    __global const int *noise_meta,
    __global const double *noise_amplitudes,
    __global const double *params,
    __global const int *cached_h2,
    __global const int *permutations,
    __global const int *blend_meta,
    __global const double *blend_params
) {
    double v1 = vg_y_gradient(y, -64, -40, 0.0, 1.0);
    double v4 = vg_y_gradient(y, 240, 256, 1.0, 0.0);
    double v7 = vg_y_gradient(y, -64, 320, 1.5, -1.5);
    double v8 = v7 + column5;
    double v9 = vg_noise(22, (double)x * 1500.0, (double)y * 0.0, (double)z * 1500.0, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v10 = (v8 + flat6 * (v9 > 0.0 ? v9 : v9 * 0.5)) * column6;
    double v11 = vg_blended((double)x, (double)y, (double)z, blend_meta, blend_params, params, permutations);
    double qn = v10 > 0.0 ? v10 : v10 * 0.25;
    double v12 = 4.0 * qn + v11;
    double v13 = vg_noise(9, (double)x * 0.75, (double)y * 0.5, (double)z * 0.75, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v15 = vg_noise(52, (double)x, (double)y, (double)z, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v16 = vg_noise(51, (double)x, (double)y, (double)z, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v17 = (-0.05 + -0.05 * v15) * (-0.4 + fabs(v16));
    double v18 = vg_noise(49, (double)x * 2.0, (double)y, (double)z * 2.0, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double rarity = vg_rarity1(v18);
    double v20 = rarity * fabs(vg_noise(47, (double)x / rarity, (double)y / rarity, (double)z / rarity, noise_meta, noise_amplitudes, params, cached_h2, permutations));
    double v21 = rarity * fabs(vg_noise(48, (double)x / rarity, (double)y / rarity, (double)z / rarity, noise_meta, noise_amplitudes, params, cached_h2, permutations));
    double v22 = vg_noise(50, (double)x, (double)y, (double)z, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v23 = vg_clamp(vg_max(v20, v21) + (-0.0765 + -0.011499999999999996 * v22), -1.0, 1.0);
    double v24 = vg_min(0.37 + v13 + vg_y_gradient(y, -10, 30, 0.3, 0.0), v17 + v23);
    double v26 = vg_noise(10, (double)x, (double)y * 8.0, (double)z, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v27 = vg_noise(8, (double)x, (double)y * 0.6666666666666666, (double)z, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v29 = vg_clamp(0.27 + v27, -1.0, 1.0) + vg_clamp(1.5 + -0.64 * v12, 0.0, 0.5);
    double v30 = vg_noise(45, (double)x * 2.0, (double)y, (double)z * 2.0, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    rarity = vg_rarity2(v30);
    double v31 = rarity * fabs(vg_noise(43, (double)x / rarity, (double)y / rarity, (double)z / rarity, noise_meta, noise_amplitudes, params, cached_h2, permutations));
    double v32 = vg_noise(46, (double)x * 2.0, (double)y, (double)z * 2.0, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v33 = -0.95 + -0.35000000000000003 * v32;
    double v35 = vg_noise(44, (double)x, (double)y * 0.0, (double)z, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v36 = fabs(8.0 * v35 + vg_y_gradient(y, -64, 320, 8.0, -40.0));
    double cube = v36 + v33;
    cube = cube * cube * cube;
    double v37 = vg_clamp(vg_max(v31 + 0.083 * v33, cube), -1.0, 1.0);
    double v38 = vg_noise(37, (double)x * 25.0, (double)y * 0.3, (double)z * 25.0, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v40 = vg_noise(38, (double)x, (double)y, (double)z, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v42 = vg_noise(39, (double)x, (double)y, (double)z, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double thickness = 0.55 + 0.55 * v42;
    double v43 = (2.0 * v38 + (-1.0 + -1.0 * v40)) * (thickness * thickness * thickness);
    double v45 = (v43 >= -1000000.0 && v43 < 0.03) ? -1000000.0 : v43;
    double v46;
    if (v12 >= -1000000.0 && v12 < 1.5625) {
        v46 = vg_min(v12, 5.0 * v24);
    } else {
        v46 = vg_max(vg_min(vg_min(4.0 * (v26 * v26) + v29, v24), v37 + v17), v45);
    }
    double v47 = vg_y_gradient(y, -4064, 4062, -4064.0, 4062.0);
    double v48 = vg_noise(26, (double)x, (double)y, (double)z, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v49 = (v47 >= -60.0 && v47 < 321.0) ? v48 : -1.0;
    double v50 = vg_noise(29, (double)x, (double)y, (double)z, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v51 = (v47 >= -60.0 && v47 < 321.0) ? (-0.07500000000000001 + -0.025 * v50) : 0.0;
    double v52 = vg_noise(27, (double)x * 2.6666666666666665, (double)y * 2.6666666666666665, (double)z * 2.6666666666666665, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v53 = (v47 >= -60.0 && v47 < 321.0) ? v52 : 0.0;
    double v54 = vg_noise(28, (double)x * 2.6666666666666665, (double)y * 2.6666666666666665, (double)z * 2.6666666666666665, noise_meta, noise_amplitudes, params, cached_h2, permutations);
    double v55 = (v47 >= -60.0 && v47 < 321.0) ? v54 : 0.0;
    double v56 = (v49 >= -1000000.0 && v49 < 0.0) ? 64.0 : (v51 + 1.5 * vg_max(fabs(v53), fabs(v55)));
    double base = 0.1171875 + v1 * (-0.1171875 + (-0.078125 + v4 * (0.078125 + v46)));
    double c = vg_clamp(0.64 * base, -1.0, 1.0);
    return vg_min(c / 2.0 - c * c * c / 24.0, v56);
}

/*__SPLINE_FUNCTIONS__*/

__kernel void vg_final_density_kernel(
    __global const int *noise_meta,
    __global const double *noise_amplitudes,
    __global const double *perlin_params,
    __global const int *cached_h2,
    __global const int *permutations,
    __global const int *blend_meta,
    __global const double *blend_params,
    __global const int *chunk_meta,
    __global const double *column_inputs,
    __global double *output
) {
    int gid = (int)get_global_id(0);
    int corner_y_count = chunk_meta[3];
    int column = gid / corner_y_count;
    int corner_y = gid - column * corner_y_count;
    int corner_x = column / 5;
    int corner_z = column - corner_x * 5;
    int x = chunk_meta[0] + corner_x * 4;
    int z = chunk_meta[1] + corner_z * 4;
    int y = chunk_meta[2] + corner_y * 8;
    int ci = column * 3;
    output[gid] = vg_final_density(x, y, z, column_inputs[ci], column_inputs[ci + 1], column_inputs[ci + 2], noise_meta, noise_amplitudes, perlin_params, cached_h2, permutations, blend_meta, blend_params);
}

// Climate's expensive noise fields only depend on X/Z. The host groups
// consecutive points into columns and expands the cheap Y-dependent depth
// gradient itself. A normal feature-biome query therefore evaluates 144
// columns instead of repeating the same noise work for 13,968 points.
__kernel void vg_climate_kernel(
    __global const int *noise_meta,
    __global const double *noise_amplitudes,
    __global const double *perlin_params,
    __global const int *cached_h2,
    __global const int *permutations,
    __global const int *columns,
    __global double *output
) {
    int gid = (int)get_global_id(0);
    int x = columns[gid * 2];
    int z = columns[gid * 2 + 1];
    double shift_x = vg_noise(30, (double)x * 0.25, 0.0, (double)z * 0.25, noise_meta, noise_amplitudes, perlin_params, cached_h2, permutations) * 4.0;
    double shift_z = vg_noise(30, (double)z * 0.25, (double)x * 0.25, 0.0, noise_meta, noise_amplitudes, perlin_params, cached_h2, permutations) * 4.0;
    double sx = (double)x * 0.25 + shift_x;
    double sz = (double)z * 0.25 + shift_z;
    double continental = vg_noise(12, sx, 0.0, sz, noise_meta, noise_amplitudes, perlin_params, cached_h2, permutations);
    double erosion = vg_noise(14, sx, 0.0, sz, noise_meta, noise_amplitudes, perlin_params, cached_h2, permutations);
    double ridges = vg_noise(41, sx, 0.0, sz, noise_meta, noise_amplitudes, perlin_params, cached_h2, permutations);
    double temperature = vg_noise(56, sx, 0.0, sz, noise_meta, noise_amplitudes, perlin_params, cached_h2, permutations);
    double vegetation = vg_noise(58, sx, 0.0, sz, noise_meta, noise_amplitudes, perlin_params, cached_h2, permutations);
    double peaks_valleys = -3.0 * (fabs(fabs(ridges) - 0.6666666666666666) - 0.3333333333333333);
    double offset = -0.5037500262260437 + (double)vg_terrain_offset(continental, erosion, ridges, peaks_valleys);
    int out = gid * 6;
    output[out] = temperature;
    output[out + 1] = vegetation;
    output[out + 2] = continental;
    output[out + 3] = erosion;
    output[out + 4] = offset;
    output[out + 5] = ridges;
}

inline long vg_quantize_climate(double value) {
    return (long)((float)value * 10000.0f);
}

inline long vg_climate_tree_distance(
    int node,
    __private const long *target,
    __global const long *tree_spaces
) {
    int base = node * 14;
    long total = 0;
    for (int dimension = 0; dimension < 7; dimension++) {
        long minimum = tree_spaces[base + dimension * 2];
        long maximum = tree_spaces[base + dimension * 2 + 1];
        long value = target[dimension];
        long delta = value < minimum ? minimum - value : (value > maximum ? value - maximum : 0);
        total += delta * delta;
    }
    return total;
}

// This is the same depth-first, strict-less-than traversal as
// climateRTreeNode.search. Children are visited in their original order so
// equal-fitness tie outcomes remain bit-for-bit identical to the CPU path.
__kernel void vg_biome_kernel(
    __global const int *tree_meta,
    __global const long *tree_spaces,
    __global const uchar *tree_biomes,
    __global const int *points,
    __global const double *columns,
    __global uchar *output
) {
    int gid = (int)get_global_id(0);
    int y = points[gid * 2];
    int column = points[gid * 2 + 1];
    int base = column * 6;
    double depth = vg_y_gradient(y, -64, 320, 1.5, -1.5) + columns[base + 4];
    long target[7] = {
        vg_quantize_climate(columns[base]),
        vg_quantize_climate(columns[base + 1]),
        vg_quantize_climate(columns[base + 2]),
        vg_quantize_climate(columns[base + 3]),
        vg_quantize_climate(depth),
        vg_quantize_climate(columns[base + 5]),
        0
    };

    long best_distance = 0x7fffffffffffffffL;
    uchar best_biome = 1;
    int node_stack[16];
    int child_stack[16];
    int depth_index = 0;
    node_stack[0] = 0;
    child_stack[0] = 0;

    while (depth_index >= 0) {
        int node = node_stack[depth_index];
        int first_child = tree_meta[node * 2];
        int child_count = tree_meta[node * 2 + 1];
        if (child_count == 0) {
            long distance = vg_climate_tree_distance(node, target, tree_spaces);
            if (best_distance > distance) {
                best_distance = distance;
                best_biome = tree_biomes[node];
            }
            depth_index--;
            continue;
        }

        int next_child = child_stack[depth_index];
        if (next_child >= child_count) {
            depth_index--;
            continue;
        }
		child_stack[depth_index] = next_child + 1;
		int child = first_child + next_child;
		long child_distance = vg_climate_tree_distance(child, target, tree_spaces);
		if (best_distance <= child_distance) {
			continue;
		}
		int child_children = tree_meta[child * 2 + 1];
		if (child_children == 0) {
			if (best_distance > child_distance) {
				best_distance = child_distance;
                best_biome = tree_biomes[child];
            }
            continue;
        }
        depth_index++;
        node_stack[depth_index] = child;
        child_stack[depth_index] = 0;
    }
    output[gid] = best_biome;
}
