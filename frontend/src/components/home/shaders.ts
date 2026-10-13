// GLSL for the homepage particle universe (WebGL 1 so it runs nearly everywhere).

export const PARTICLE_VERTEX = `
precision highp float;

attribute vec4 aSeed;
attribute vec4 aA;
attribute vec4 aB;
attribute vec4 aC;

uniform mat4 uProj;
uniform mat4 uView;
uniform float uTime;

// mode, spin, tiltX, tiltZ
uniform vec4 uMotionA;
uniform vec4 uMotionB;
uniform vec4 uMotionC;
// size, alpha, jitter, unused
uniform vec4 uLookA;
uniform vec4 uLookB;
uniform vec4 uLookC;

uniform float uMix;
uniform float uIntro;
uniform float uEnergy;

uniform vec3 uPointer;
uniform vec4 uRipple;
uniform float uAspect;
uniform float uSize;
uniform float uWarp;
uniform float uFade;

uniform vec3 uPal0;
uniform vec3 uPal1;
uniform vec3 uPal2;
uniform vec3 uPal3;
uniform vec3 uAccent;
uniform vec3 uBright;

varying vec3 vColor;
varying float vAlpha;

vec2 rot2(vec2 v, float a) {
  float c = cos(a);
  float s = sin(a);
  return vec2(c * v.x - s * v.y, s * v.x + c * v.y);
}

vec3 animate(vec4 p, vec4 motion, vec4 look, out float bright, out float tone) {
  tone = floor(p.w / 16.0 + 0.0005);
  float val = p.w - tone * 16.0;
  vec3 q = p.xyz;
  bright = 1.0;
  if (motion.x > 0.5 && motion.x < 1.5) {
    // Keplerian orbit: each particle carries its own angular speed.
    q.xz = rot2(q.xz, uTime * val);
  } else if (motion.x > 1.5) {
    // Warp tunnel: particles stream toward the camera and fade at both ends.
    q.z = mod(q.z + uTime * val + 34.0, 42.0) - 34.0;
    bright = smoothstep(-34.0, -20.0, q.z) * (1.0 - smoothstep(3.5, 8.0, q.z));
  }
  if (look.z > 0.0) {
    q += look.z * vec3(
      sin(uTime * (0.6 + aSeed.x) + aSeed.y * 6.2831),
      cos(uTime * (0.5 + aSeed.z) + aSeed.x * 6.2831),
      sin(uTime * (0.7 + aSeed.y) + aSeed.z * 6.2831));
  }
  q.xz = rot2(q.xz, uTime * motion.y);
  q.yz = rot2(q.yz, motion.z);
  q.xy = rot2(q.xy, motion.w);
  return q;
}

void main() {
  float st = aSeed.x * 0.4;
  float mF = smoothstep(st, st + 0.6, uMix);
  float mI = smoothstep(st, st + 0.6, uIntro);

  float bA; float tA;
  vec3 pos = animate(aA, uMotionA, uLookA, bA, tA);
  float bright = bA;
  float tone = tA;
  vec4 look = uLookA;
  if (uMix > 0.0001) {
    float bB; float tB;
    vec3 pB = animate(aB, uMotionB, uLookB, bB, tB);
    pos = mix(pos, pB, mF);
    bright = mix(bright, bB, mF);
    look = mix(look, uLookB, mF);
    tone = mF < 0.5 ? tone : tB;
  }
  if (uIntro > 0.0001) {
    float bC; float tC;
    vec3 pC = animate(aC, uMotionC, uLookC, bC, tC);
    pos = mix(pos, pC, mI);
    bright = mix(bright, bC, mI);
    look = mix(look, uLookC, mI);
    tone = mI < 0.5 ? tone : tC;
  }

  // Particles burst outwards and swirl while they travel between formations.
  float energy = (mF * (1.0 - mF) + mI * (1.0 - mI)) * 4.0 * uEnergy;
  vec3 dir = normalize(aSeed.yzw - 0.5 + vec3(0.0001));
  pos += dir * energy * (0.3 + aSeed.x * 0.9);
  pos.xz = rot2(pos.xz, energy * (aSeed.y - 0.35) * 0.7);

  vec4 mv = uView * vec4(pos, 1.0);
  mv.z += uWarp * (1.2 + aSeed.y * 4.5);
  vec4 clip = uProj * mv;

  float infl = 0.0;
  if (clip.w > 0.0) {
    vec2 ndc = clip.xy / clip.w;
    vec2 offs = vec2(0.0);
    if (uPointer.z > 0.001) {
      // Cursor: push particles away and swirl them around the pointer.
      vec2 d = ndc - uPointer.xy;
      d.x *= uAspect;
      float dist = length(d);
      infl = uPointer.z * (1.0 - smoothstep(0.0, 0.38, dist));
      vec2 n = dist > 0.0001 ? d / dist : vec2(0.0);
      offs += n * infl * 0.085 + vec2(-n.y, n.x) * infl * (0.035 + aSeed.w * 0.04);
    }
    if (uRipple.w > 0.001) {
      // Click: a shockwave ring expands from the click point.
      vec2 d = ndc - uRipple.xy;
      d.x *= uAspect;
      float dist = length(d);
      float band = 1.0 - smoothstep(0.0, 0.14, abs(dist - uRipple.z * 1.35));
      float k = band * uRipple.w;
      vec2 n = dist > 0.0001 ? d / dist : vec2(0.0);
      offs += n * k * (0.05 + aSeed.y * 0.05);
      infl = max(infl, k);
    }
    offs.x /= uAspect;
    clip.xy += offs * clip.w;
  }
  gl_Position = clip;

  float depth = max(-mv.z, 0.05);
  float size = uSize * look.x * (0.42 + aSeed.y * aSeed.y * 1.45) / depth;
  size *= 1.0 + infl * 0.9 + uWarp * 1.1;
  gl_PointSize = clamp(size, 1.0, 40.0);

  vec3 col = aSeed.z < 0.42 ? uPal0 : (aSeed.z < 0.72 ? uPal1 : (aSeed.z < 0.9 ? uPal2 : uPal3));
  if (tone > 0.5 && tone < 1.5) col = uAccent;
  else if (tone > 1.5) col = uBright;

  float twinkle = 0.7 + 0.3 * sin(uTime * (0.6 + aSeed.w * 2.4) + aSeed.x * 43.0);
  float fog = 1.0 - smoothstep(12.0, 54.0, depth);
  float nearFade = smoothstep(0.35, 2.0, depth);
  vAlpha = clamp(look.y * bright * twinkle * fog * nearFade * uFade * (1.0 + infl * 0.9) * min(size, 1.0), 0.0, 1.0);
  vColor = col;
}
`

export const PARTICLE_FRAGMENT = `
precision mediump float;

varying vec3 vColor;
varying float vAlpha;
uniform float uGlow;

void main() {
  vec2 c = gl_PointCoord * 2.0 - 1.0;
  float d = dot(c, c);
  if (d > 1.0) discard;
  float soft = 1.0 - d;
  soft *= soft;
  float hot = (1.0 - smoothstep(0.0, 0.16, d)) * uGlow;
  vec3 col = min(vColor + hot * 0.55, vec3(1.0));
  float a = vAlpha * soft;
  gl_FragColor = vec4(col * a, a);
}
`

export const TITLE_VERTEX = `
precision highp float;
attribute vec2 aCorner;
uniform mat4 uProj;
uniform mat4 uView;
uniform vec3 uCenter;
uniform vec2 uHalf;
varying vec2 vUv;
void main() {
  vUv = vec2(aCorner.x * 0.5 + 0.5, 0.5 - aCorner.y * 0.5);
  vec3 p = uCenter + vec3(aCorner.x * uHalf.x, aCorner.y * uHalf.y, 0.0);
  gl_Position = uProj * uView * vec4(p, 1.0);
}
`

export const TITLE_FRAGMENT = `
precision mediump float;
uniform sampler2D uTex;
uniform float uOpacity;
uniform float uReveal;
uniform float uCut;
varying vec2 vUv;
void main() {
  vec4 t = texture2D(uTex, vUv);
  // A soft left-to-right sweep while the wordmark arrives.
  float sweep = 1.0 - smoothstep(uReveal - 0.18, uReveal, vUv.x * 0.82 + 0.09);
  float k = uOpacity * sweep;
  if (t.a * k < uCut) discard;
  gl_FragColor = t * k;
}
`
