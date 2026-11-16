package volume

// The volume ray-march fragment shader is shared by the desktop (OpenGL 3.3)
// and browser (WebGL2) back ends. Each back end prepends a header that sets
// the GLSL version and defines three macros:
//
//	TF_SAMPLER     sampler type of the transfer-function texture
//	TF_SAMPLE(n)   transfer-function lookup at normalized density n
//	DECODE_GRAD(v) gradient vector from the volume texture's GBA channels
//
// Writing the march once keeps the two renderers from drifting apart.

// desktopFragmentHeader: 1D transfer function, gradients stored around 0.5 in
// RGBA16 normalized texels.
const desktopFragmentHeader = `#version 330 core
#define TF_SAMPLER sampler1D
#define TF_SAMPLE(n) texture(transferTex, n)
#define DECODE_GRAD(v) ((v) * 2.0 - 1.0)
`

// webFragmentHeader: WebGL2 has no 1D textures and no RGBA16 normalized
// format, so the transfer function is an N x 1 2D texture and gradients are
// stored signed in RGBA16F texels.
const webFragmentHeader = `#version 300 es
precision highp float;
precision highp sampler3D;
#define TF_SAMPLER sampler2D
#define TF_SAMPLE(n) texture(transferTex, vec2(n, 0.5))
#define DECODE_GRAD(v) (v)
`

const volumeFragmentBody = `in vec2 TexCoord;
out vec4 FragColor;

uniform sampler3D volumeTex;
uniform TF_SAMPLER transferTex;
uniform vec3 camPos;
uniform vec3 camForward;
uniform vec3 camRight;
uniform vec3 camUp;
uniform float fov;
uniform float aspectRatio;
uniform float windowMin;
uniform float windowRange;
uniform float alphaScale;
uniform float stepSize;
uniform float scaleZ;
uniform float rescaleIntercept;
uniform vec4 tintColor;
uniform float densityThreshold;
uniform float ambientIntensity;
uniform float diffuseIntensity;
uniform float specularIntensity;
uniform vec4 clipPlane;
uniform bool enableClip;
uniform float depthCue;

// Ray-box intersection for box [0,1]x[0,1]x[0,scaleZ]
vec2 intersectBox(vec3 origin, vec3 dir) {
    vec3 boxSize = vec3(1.0, 1.0, scaleZ);
    vec3 invDir = 1.0 / dir;
    vec3 t1 = (vec3(0.0) - origin) * invDir;
    vec3 t2 = (boxSize - origin) * invDir;
    vec3 tMin = min(t1, t2);
    vec3 tMax = max(t1, t2);
    float tNear = max(max(tMin.x, tMin.y), tMin.z);
    float tFar = min(min(tMax.x, tMax.y), tMax.z);
    return vec2(tNear, tFar);
}

void main() {
    vec2 uv = TexCoord * 2.0 - 1.0;
    vec3 rayDir = normalize(camForward + camRight * uv.x * fov * aspectRatio + camUp * uv.y * fov);

    // Intersect with volume bounding box
    vec2 tHit = intersectBox(camPos, rayDir);
    float tNear = max(tHit.x, 0.0);
    float tFar = tHit.y;

    if (tFar < tNear) {
        FragColor = vec4(0.0);
        return;
    }

    vec4 accum = vec4(0.0);
    float t = tNear;
    vec3 boxSize = vec3(1.0, 1.0, scaleZ);

    // Light directions for better depth perception
    vec3 lightDir1 = normalize(vec3(0.5, 0.7, 1.0));   // Main light (front-top-right)
    vec3 lightDir2 = normalize(vec3(-0.3, 0.2, 0.8));  // Fill light (front-top-left)

    for (int i = 0; i < 4096; i++) {
        if (t >= tFar || accum.a > 0.98) break;

        vec3 pos = camPos + rayDir * t;

        // Sample normalized coordinate
        vec3 texPos = pos / boxSize;

        // Clipping plane check (in texture space 0..1)
        if (enableClip) {
             if (dot(texPos, clipPlane.xyz) + clipPlane.w < 0.0) {
                 t += stepSize;
                 continue;
             }
        }

        // Single texture read: density (R) + pre-computed gradient (GBA)
        vec4 volSample = texture(volumeTex, texPos);
        float density = volSample.r;

        // Window/level normalization
        float normalized = clamp((density * 65535.0 + rescaleIntercept - windowMin) / windowRange, 0.0, 1.0);

        if (normalized > densityThreshold) {
            vec4 sampleColor = TF_SAMPLE(normalized);

            // Alpha threshold skip: avoid lighting for nearly-transparent samples
            if (sampleColor.a < 0.001) {
                t += stepSize;
                continue;
            }

            // Apply tint
            sampleColor.rgb *= tintColor.rgb;

            // Decode pre-computed gradient from GBA channels (encoding is per back end)
            vec3 gradient = DECODE_GRAD(volSample.gba);
            float gradLen = length(gradient);

            // Edge enhancement: boost opacity at surfaces (high gradient)
            float edgeFactor = smoothstep(0.001, 0.02, gradLen);

            // Only compute full Phong lighting when gradient is significant
            if (gradLen > 0.01) {
                vec3 normal = normalize(gradient);

                // Two-light Phong lighting for better depth
                float NdotL1 = max(dot(normal, lightDir1), 0.0);
                float NdotL2 = max(dot(normal, lightDir2), 0.0);

                // Specular highlight (Blinn-Phong)
                vec3 viewDir = -rayDir;
                vec3 halfVec1 = normalize(lightDir1 + viewDir);
                vec3 halfVec2 = normalize(lightDir2 + viewDir);
                float spec1 = pow(max(dot(normal, halfVec1), 0.0), 32.0);
                float spec2 = pow(max(dot(normal, halfVec2), 0.0), 32.0);

                // Combined lighting
                float ambient = ambientIntensity;
                float diffuse = diffuseIntensity * (NdotL1 * 0.7 + NdotL2 * 0.3);
                float specular = specularIntensity * (spec1 * 0.7 + spec2 * 0.3);

                float lightFactor = ambient + diffuse;
                sampleColor.rgb *= clamp(lightFactor, 0.4, 1.4);
                sampleColor.rgb += vec3(specular * 0.3); // Subtle specular highlight

                // Edge enhancement: increase opacity at boundaries
                sampleColor.a *= (1.0 + edgeFactor * 0.5);
            } else {
                // Flat region: ambient-only lighting
                sampleColor.rgb *= ambientIntensity;
            }

            // Depth cue: darken samples further along the ray (0 = off).
            if (depthCue > 0.0) {
                float depthFrac = clamp((t - tNear) / max(tFar - tNear, 1e-4), 0.0, 1.0);
                sampleColor.rgb *= 1.0 - depthCue * depthFrac;
            }

            // Smoother opacity curve (cube root for softer falloff, then square for surfaces)
            float alphaAdjust = pow(sampleColor.a, 1.5);
            float sampleA = alphaAdjust * alphaScale;

            if (sampleA > 0.0005) {
                float oneMinusAccA = 1.0 - accum.a;
                accum.rgb += oneMinusAccA * sampleA * sampleColor.rgb;
                accum.a += oneMinusAccA * sampleA;
            }
        }

        t += stepSize;
    }

    FragColor = accum;
}
`
