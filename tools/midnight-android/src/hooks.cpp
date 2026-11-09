#include <dlfcn.h>
#include <cstdio>
#include <cstring>
#include <cstdint>
#include "includes/hooks.h"
#include "includes/dobby/dobby.h"
#include "includes/logger.h"
#include "includes/url.h"
#include "includes/opts.h"
#include "includes/celestial.h"

static void* GetLibBase(const char* libName) {
    FILE* fp = fopen("/proc/self/maps", "r");
    if (!fp) return nullptr;

    char line[512];
    while (fgets(line, sizeof(line), fp)) {
        if (strstr(line, libName)) {
            uintptr_t base = strtoull(line, nullptr, 16);
            fclose(fp);
            return (void*)base;
        }
    }

    fclose(fp);
    return nullptr;
}

// ud hooks
namespace Hooks {
    namespace CurlHook {
        int (*OG_SETOPT_UE)(void*, int, void*)  = nullptr;
        int (*OG_SETOPT_EOS)(void*, int, void*) = nullptr;

        // Shared setopt logic — takes the correct OG for whichever lib was hooked.
        static int DoSetOpt(int (*OG)(void*, int, void*), void* handle, int option, void* args) {
            if (!OG)
                return -1;

            if (option == 10002 && args != nullptr) { // CURLOPT_URL
                const char* UrlCStr = static_cast<const char*>(args);
                if (UrlCStr) {
                    Url url = Url::ParseUrl(UrlCStr);
                    const char* target = Url::GetRedirectTarget(url.host);
                    // Build the outgoing URL: redirect to our backend if matched, else leave as-is.
                    std::string finalUrl = target ? Url::CreateUrl(target, url.pathAndQuery) : std::string(UrlCStr);
                    // Celestial: on the Quest login, append the trusted token (JWT is URL-safe).
                    if (Celestial::NeedsToken(url.pathAndQuery)) {
                        std::string tok = Celestial::Token(8000);
                        if (!tok.empty()) {
                            finalUrl += (finalUrl.find('?') == std::string::npos ? '?' : '&');
                            finalUrl += "celestialToken=";
                            finalUrl += tok;
                        } else {
                            LOGE("Celestial: no token yet for login request");
                        }
                    }
                    if (finalUrl != UrlCStr) {
                        thread_local std::string newUrl;
                        newUrl = finalUrl;
                        LOGI("Redirected Request from %s to %s", UrlCStr, newUrl.c_str());
                        return OG(handle, option, (void*)newUrl.c_str());
                    }
                }
            } else if (option == 64 && BYPASS_SSL) { // CURLOPT_SSL_VERIFYPEER -> 0
                return OG(handle, option, reinterpret_cast<void*>(static_cast<intptr_t>(0)));
            }

            return OG(handle, option, args);
        }

        int SetOptHookUE(void* handle, int option, void* args)  { return DoSetOpt(OG_SETOPT_UE,  handle, option, args); }
        int SetOptHookEOS(void* handle, int option, void* args) { return DoSetOpt(OG_SETOPT_EOS, handle, option, args); }

        void* SetupHook(void*) {
            // curl_easy_setopt is statically-linked + stripped in A2 (not exported), so resolve by
            // offset. Offsets found via caller-tally (sites doing `mov w1, #CURLOPT_URL(10002); bl X`
            // uniquely identify curl_easy_setopt) cross-checked against the variadic `va_list`
            // prologue + `if(!curl) return CURLE_BAD_FUNCTION_ARGUMENT(43)` idiom.
            //
            // A2 1.0.50451 (Android, arm64-v8a): the EOS SDK is now statically MERGED into
            // libUnreal.so, so there is NO separate libEOSSDK.so. Both curl_easy_setopt copies
            // (UE's HTTP curl + EOS SDK's bundled curl, two distinct static libcurl/8.4.0 builds)
            // live inside libUnreal.so. Hook BOTH so UE *and* EOS backend traffic is redirected.
            struct Target { const char* lib; uintptr_t off; void* hook; void** og; };
            Target targets[] = {
                // UE curl_easy_setopt  (thin variadic wrapper -> Curl_vsetopt). prev: libUnreal.so+0x96aa490
                { "libUnreal.so", 0xa2b84bc, (void*)SetOptHookUE,  (void**)&OG_SETOPT_UE  },
                // EOS curl_easy_setopt (Curl_vsetopt inlined, big option switch). prev: libEOSSDK.so+0x13fa898
                { "libUnreal.so", 0xa324f04, (void*)SetOptHookEOS, (void**)&OG_SETOPT_EOS },
            };

            for (auto& t : targets) {
                // ensure the lib is mapped
                dlopen(t.lib, 2);
                void* base = GetLibBase(t.lib);
                if (!base) {
                    LOGE("Failed to find base of %s", t.lib);
                    continue;
                }
                void* fn = (void*)((char*)base + t.off);
                if (DobbyHook(fn, t.hook, t.og) == 0) {
                    LOGI("Successfully hooked curl_easy_setopt in %s @ %p", t.lib, fn);
                } else {
                    LOGE("Failed to hook curl_easy_setopt in %s @ %p", t.lib, fn);
                }
            }

            return nullptr;
        }
    }

    bool (*FCurlHook::OGProcessRequest)(void*) = nullptr;
    void (*FCurlHook::SetUrl)(void*, const void*) = nullptr;

    void* FCurlHook::SetupHook(void* handle) {
        auto libName = "libUnreal.so"; // idk change this to libUE4.so if ur doing s18
        handle = dlopen(libName, 2);
        if (!handle) return nullptr;

        void* base = GetLibBase(libName);
        if (!base) return nullptr;

        SetUrl = (void (*)(void*, const void*))((char*)base + FCurlHookOpts::SETURL_ADDR);
        void* processRequestPtr = (char*)base + FCurlHookOpts::PR_ADDR;
        if (DobbyHook(processRequestPtr, (void*)FCurlHook::ProcessRequest, (void**)&OGProcessRequest) == 0) {
            LOGI("Successfully hooked Unreal's ProcessRequest!");
        } else {
            LOGE("Failed to hook Unreal's ProcessRequest.");
        }

        return nullptr;
    }

    bool (*EOSFCurlHook::OGProcessRequest)(void*) = nullptr;
    void (*EOSFCurlHook::SetUrl)(void*, const void*) = nullptr;

    void* EOSFCurlHook::SetupHook(void* handle) {
        auto libName = "libEOSSDK.so";
        handle = dlopen(libName, 2);
        if (!handle) return nullptr;

        void* base = GetLibBase(libName);
        if (!base) return nullptr;

        SetUrl = (void (*)(void*, const void*))((char*)base + EOSFCurlHookOpts::SETURL_ADDR);
        void* processRequestPtr = (char*)base + EOSFCurlHookOpts::PR_ADDR;
        if (DobbyHook(processRequestPtr, (void*)EOSFCurlHook::ProcessRequest, (void**)&OGProcessRequest) == 0) {
            LOGI("Successfully hooked EOS's ProcessRequest!");
        } else {
            LOGE("Failed to hook EOS's ProcessRequest.");
        }

        return nullptr;
    }

    namespace InternalHooks {
        bool InternalProcessRequest(void* Req, bool isEOS) {
            if (!Req) return false;

            // getting functions
            auto OGProcessRequest = isEOS ? EOSFCurlHook::OGProcessRequest : FCurlHook::OGProcessRequest;
            auto SetUrl = isEOS ? EOSFCurlHook::SetUrl : FCurlHook::SetUrl;

            // scuffed url getter
            FString::FString& OGUrl = isEOS ? ((EOSFCurlHook*)Req)->GetUrl() : ((FCurlHook*)Req)->GetUrl();
            if (OGUrl.length == 0 || !OGUrl.data)
                return OGProcessRequest(Req);

            char* OGUrlCStr = FString::ToCStr(&OGUrl);
            if (!OGUrlCStr) return OGProcessRequest(Req);

            // proper url setter
            Url url = Url::ParseUrl(OGUrlCStr);
            if (const char* target = Url::GetRedirectTarget(url.host)) {
                std::string newUrlCStr = Url::CreateUrl(target, url.pathAndQuery);
                LOGI("Redirected Request from %s to %s", OGUrlCStr, newUrlCStr.c_str());
                auto* newUrl = (FString::FString*)malloc(sizeof(FString::FString));
                *newUrl = FString::FromCStr(newUrlCStr.c_str());
                SetUrl(Req, newUrl);
            }

            free(OGUrlCStr);
            return OGProcessRequest(Req);
        }
    }
}