#pragma once
#include <string>
#include <array>
#include <cstring>
#include "includes/opts.h"

struct Url {
    std::string host;
    std::string pathAndQuery;

    Url() = default;
    Url(std::string h, std::string pq) : host(std::move(h)), pathAndQuery(std::move(pq)) {}

    static Url ParseUrl(const std::string& url) {
        std::string host;
        std::string pathAndQuery;

        if (url.empty()) {
            return Url{host, pathAndQuery};
        }

        const std::string proto_sep = "://";
        auto proto_end = url.find(proto_sep);

        if (proto_end != std::string::npos) {
            // protocol
            host.append(url, 0, proto_end);
            host.append("://");

            const std::string remainder = url.substr(proto_end + proto_sep.size());
            auto path_start = remainder.find('/');

            if (path_start != std::string::npos) {
                // address/domain + port
                host.append(remainder, 0, path_start);
                // path + query
                pathAndQuery.append(remainder.substr(path_start));
            } else {
                host.append(remainder);
            }
        } else {
            // No protocol wow
            auto path_start = url.find('/');

            if (path_start != std::string::npos) {
                host.append(url, 0, path_start);
                pathAndQuery.append(url.substr(path_start));
            } else {
                host.append(url);
            }
        }

        return Url{host, pathAndQuery};
    }

    // [A2] Returns the backend URL a host should be redirected to, or nullptr for no redirect.
    // Suffix-match, so ANY subdomain of the target domains is caught (e.g. foo.aa-mothership.com).
    static const char* GetRedirectTarget(const std::string& host) {
        std::string stripped = host;
        // strip scheme
        if (stripped.rfind("http://", 0) == 0) {
            stripped = stripped.substr(7);
        } else if (stripped.rfind("https://", 0) == 0) {
            stripped = stripped.substr(8);
        }
        // strip port
        auto colon = stripped.find(':');
        if (colon != std::string::npos) {
            stripped = stripped.substr(0, colon);
        }

        // Substring match, mirroring the Fiddler `hostname.Contains(...)` rules.
        auto contains = [&](const char* kw) -> bool {
            return stripped.find(kw) != std::string::npos;
        };

        if (contains("epicgames"))     return EPIC_BACKEND;       // -> :50 (EOS)
        if (contains("aa-mothership")) return MOTHERSHIP_BACKEND; // -> :90
        if (contains("oriondrift"))    return ORION_BACKEND;      // -> :78
        return nullptr;
    }

    static bool ShouldRedirect(const std::string& host) {
        return GetRedirectTarget(host) != nullptr;
    }

    static std::string CreateUrl(const std::string& host, const std::string& pathAndQuery) {
        std::string url;
        url.reserve(host.size() + pathAndQuery.size());
        url.append(host);
        url.append(pathAndQuery);
        return url;
    }
};
