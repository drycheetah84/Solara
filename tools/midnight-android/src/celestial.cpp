// Celestial client: hash the OBB, fetch the trusted token from Mothership, keep it fresh.
//
// The HTTPS request goes through Java's HttpsURLConnection via JNI, so it uses Android's own TLS
// stack and needs no curl offsets. Nothing here touches the game process — it reads one file
// (the OBB) and makes one HTTPS POST.
#include <jni.h>
#include <pthread.h>
#include <unistd.h>
#include <dirent.h>
#include <cstdio>
#include <cstdint>
#include <cstring>
#include <string>
#include <vector>
#include <mutex>
#include <condition_variable>
#include <chrono>
#include "includes/celestial.h"
#include "includes/logger.h"
#include "includes/opts.h"

// ---------------------------------------------------------------------------
// SHA-256 (public-domain style, streaming so the 2 GB OBB is read in chunks)
// ---------------------------------------------------------------------------
namespace {
    const uint32_t K256[64] = {
        0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,
        0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,
        0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,
        0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,
        0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,
        0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,
        0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,
        0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2 };

    struct Sha256 {
        uint32_t h[8]; uint64_t len = 0; uint8_t buf[64]; size_t n = 0;
        Sha256() {
            const uint32_t iv[8] = {0x6a09e667,0xbb67ae85,0x3c6ef372,0xa54ff53a,
                                    0x510e527f,0x9b05688c,0x1f83d9ab,0x5be0cd19};
            memcpy(h, iv, sizeof iv);
        }
        static uint32_t ror(uint32_t x, int r) { return (x >> r) | (x << (32 - r)); }
        void block(const uint8_t* p) {
            uint32_t w[64];
            for (int i = 0; i < 16; i++)
                w[i] = (p[i*4] << 24) | (p[i*4+1] << 16) | (p[i*4+2] << 8) | p[i*4+3];
            for (int i = 16; i < 64; i++) {
                uint32_t s0 = ror(w[i-15],7) ^ ror(w[i-15],18) ^ (w[i-15] >> 3);
                uint32_t s1 = ror(w[i-2],17) ^ ror(w[i-2],19) ^ (w[i-2] >> 10);
                w[i] = w[i-16] + s0 + w[i-7] + s1;
            }
            uint32_t a=h[0],b=h[1],c=h[2],d=h[3],e=h[4],f=h[5],g=h[6],H=h[7];
            for (int i = 0; i < 64; i++) {
                uint32_t S1 = ror(e,6) ^ ror(e,11) ^ ror(e,25);
                uint32_t ch = (e & f) ^ (~e & g);
                uint32_t t1 = H + S1 + ch + K256[i] + w[i];
                uint32_t S0 = ror(a,2) ^ ror(a,13) ^ ror(a,22);
                uint32_t maj = (a & b) ^ (a & c) ^ (b & c);
                uint32_t t2 = S0 + maj;
                H=g; g=f; f=e; e=d+t1; d=c; c=b; b=a; a=t1+t2;
            }
            h[0]+=a;h[1]+=b;h[2]+=c;h[3]+=d;h[4]+=e;h[5]+=f;h[6]+=g;h[7]+=H;
        }
        void update(const uint8_t* p, size_t sz) {
            len += sz;
            while (sz) {
                size_t take = 64 - n; if (take > sz) take = sz;
                memcpy(buf + n, p, take); n += take; p += take; sz -= take;
                if (n == 64) { block(buf); n = 0; }
            }
        }
        std::string hex() {
            uint64_t bits = len * 8;
            uint8_t one = 0x80; update(&one, 1);
            uint8_t zero = 0; while (n != 56) update(&zero, 1);
            uint8_t L[8]; for (int i = 0; i < 8; i++) L[i] = (bits >> (56 - i*8)) & 0xff;
            update(L, 8);
            char out[65];
            for (int i = 0; i < 8; i++) snprintf(out + i*8, 9, "%08x", h[i]);
            return std::string(out, 64);
        }
    };

    std::string HashObb() {
        DIR* d = opendir(CELESTIAL_OBB_DIR);
        if (!d) { LOGE("Celestial: OBB dir not found: %s", CELESTIAL_OBB_DIR); return ""; }
        std::string path;
        for (struct dirent* e; (e = readdir(d)); ) {
            std::string name = e->d_name;
            if (name.rfind("main.", 0) == 0 && name.size() > 4 && name.substr(name.size() - 4) == ".obb") {
                path = std::string(CELESTIAL_OBB_DIR) + "/" + name;
                break;
            }
        }
        closedir(d);
        if (path.empty()) { LOGE("Celestial: no main.*.obb in %s", CELESTIAL_OBB_DIR); return ""; }

        FILE* f = fopen(path.c_str(), "rb");
        if (!f) { LOGE("Celestial: cannot open OBB %s", path.c_str()); return ""; }
        Sha256 s;
        std::vector<uint8_t> chunk(1 << 20);
        for (size_t r; (r = fread(chunk.data(), 1, chunk.size(), f)) > 0; )
            s.update(chunk.data(), r);
        fclose(f);
        std::string hex = s.hex();
        LOGI("Celestial: OBB %s sha256=%s", path.c_str(), hex.c_str());
        return hex;
    }
}

// ---------------------------------------------------------------------------
// JNI HTTPS POST (java.net.HttpsURLConnection) + token state
// ---------------------------------------------------------------------------
namespace {
    JavaVM* g_vm = nullptr;
    std::mutex g_mu;
    std::condition_variable g_cv;
    std::string g_token;

    std::string jToStr(JNIEnv* env, jstring s) {
        if (!s) return "";
        const char* c = env->GetStringUTFChars(s, nullptr);
        std::string r = c ? c : "";
        env->ReleaseStringUTFChars(s, c);
        return r;
    }

    bool failed(JNIEnv* env) {
        if (env->ExceptionCheck()) { env->ExceptionDescribe(); env->ExceptionClear(); return true; }
        return false;
    }

    // POST jsonBody to url, return the response body ("" on any failure).
    std::string HttpPost(JNIEnv* env, const char* url, const std::string& jsonBody) {
        jclass urlC = env->FindClass("java/net/URL");
        jmethodID urlCtor = env->GetMethodID(urlC, "<init>", "(Ljava/lang/String;)V");
        jobject jurl = env->NewObject(urlC, urlCtor, env->NewStringUTF(url));
        if (failed(env) || !jurl) return "";

        jmethodID openConn = env->GetMethodID(urlC, "openConnection", "()Ljava/net/URLConnection;");
        jobject conn = env->CallObjectMethod(jurl, openConn);
        if (failed(env) || !conn) return "";

        jclass urlConnC = env->FindClass("java/net/URLConnection");
        jclass httpC = env->FindClass("java/net/HttpURLConnection");

        env->CallVoidMethod(conn, env->GetMethodID(httpC, "setRequestMethod", "(Ljava/lang/String;)V"),
                            env->NewStringUTF("POST"));
        env->CallVoidMethod(conn, env->GetMethodID(urlConnC, "setDoOutput", "(Z)V"), JNI_TRUE);
        env->CallVoidMethod(conn, env->GetMethodID(urlConnC, "setConnectTimeout", "(I)V"), (jint)15000);
        env->CallVoidMethod(conn, env->GetMethodID(urlConnC, "setReadTimeout", "(I)V"), (jint)15000);
        jmethodID setProp = env->GetMethodID(urlConnC, "setRequestProperty", "(Ljava/lang/String;Ljava/lang/String;)V");
        env->CallVoidMethod(conn, setProp, env->NewStringUTF("Content-Type"), env->NewStringUTF("application/json"));
        (void)failed(env);

        // write the JSON body
        jobject os = env->CallObjectMethod(conn, env->GetMethodID(urlConnC, "getOutputStream", "()Ljava/io/OutputStream;"));
        if (failed(env) || !os) return "";
        jclass osC = env->FindClass("java/io/OutputStream");
        jbyteArray body = env->NewByteArray((jsize)jsonBody.size());
        env->SetByteArrayRegion(body, 0, (jsize)jsonBody.size(), (const jbyte*)jsonBody.data());
        env->CallVoidMethod(os, env->GetMethodID(osC, "write", "([B)V"), body);
        env->CallVoidMethod(os, env->GetMethodID(osC, "close", "()V"));
        if (failed(env)) return "";

        jint code = env->CallIntMethod(conn, env->GetMethodID(httpC, "getResponseCode", "()I"));
        if (failed(env)) return "";

        jobject is = env->CallObjectMethod(conn, env->GetMethodID(urlConnC, "getInputStream", "()Ljava/io/InputStream;"));
        if (failed(env) || !is) { LOGE("Celestial: token HTTP %d", (int)code); return ""; }

        jclass isC = env->FindClass("java/io/InputStream");
        jmethodID readM = env->GetMethodID(isC, "read", "([B)I");
        jbyteArray rbuf = env->NewByteArray(4096);
        std::string resp;
        for (;;) {
            jint got = env->CallIntMethod(is, readM, rbuf);
            if (failed(env) || got <= 0) break;
            jbyte* p = env->GetByteArrayElements(rbuf, nullptr);
            resp.append((const char*)p, got);
            env->ReleaseByteArrayElements(rbuf, p, JNI_ABORT);
        }
        env->CallVoidMethod(is, env->GetMethodID(isC, "close", "()V"));
        LOGI("Celestial: token HTTP %d, %zu bytes", (int)code, resp.size());
        return resp;
    }

    // pull "celestialToken":"..." out of the JSON response (no JSON lib needed)
    std::string ExtractToken(const std::string& json) {
        const char* key = "\"celestialToken\"";
        size_t k = json.find(key);
        if (k == std::string::npos) return "";
        size_t q1 = json.find('"', k + strlen(key) + 1);
        if (q1 == std::string::npos) return "";
        size_t q2 = json.find('"', q1 + 1);
        if (q2 == std::string::npos) return "";
        return json.substr(q1 + 1, q2 - q1 - 1);
    }

    void* Worker(void*) {
        std::string obb = HashObb();           // once; the OBB does not change while installed
        std::string body = "{\"obb\":\"" + obb + "\"}";
        for (;;) {
            JNIEnv* env = nullptr;
            int wait = 60;                     // retry a failed fetch in a minute
            if (g_vm->AttachCurrentThread(&env, nullptr) == JNI_OK) {
                std::string tok = ExtractToken(HttpPost(env, CELESTIAL_TOKEN_URL, body));
                g_vm->DetachCurrentThread();
                if (!tok.empty()) {
                    { std::lock_guard<std::mutex> lk(g_mu); g_token = tok; }
                    g_cv.notify_all();
                    LOGI("Celestial: token refreshed");
                    wait = 90 * 60;            // server TTL is 2h; refresh well before it
                }
            }
            sleep(wait);
        }
        return nullptr;
    }
}

// ---------------------------------------------------------------------------
// public API
// ---------------------------------------------------------------------------
namespace Celestial {
    void Start() {
        pthread_t t;
        pthread_create(&t, nullptr, Worker, nullptr);
    }

    std::string Token(int timeoutMs) {
        std::unique_lock<std::mutex> lk(g_mu);
        if (g_token.empty() && timeoutMs > 0)
            g_cv.wait_for(lk, std::chrono::milliseconds(timeoutMs), [] { return !g_token.empty(); });
        return g_token;
    }

    bool NeedsToken(const std::string& pathAndQuery) {
        return pathAndQuery.find("/player/client/auth/complete/QUEST") != std::string::npos;
    }
}

extern "C" JNIEXPORT jint JNICALL JNI_OnLoad(JavaVM* vm, void*) {
    g_vm = vm;
    Celestial::Start();
    return JNI_VERSION_1_6;
}
