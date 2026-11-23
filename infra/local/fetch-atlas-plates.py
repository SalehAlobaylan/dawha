#!/usr/bin/env python3
"""Fetch and verify the additional historical atlas plates referenced by db/seeds/010_additional_atlas_plates.sql."""

import argparse
import hashlib
import json
import sys
import urllib.parse
import urllib.request
from pathlib import Path

UA = "DawhaAtlasFetcher/1.0 (https://github.com/SalehAlobaylan/dawha)"

PLATES = [
    {
        "filename": "admiralty-1916-districts.jpg",
        "commons_title": "File:1916 map by the British Admiralty War Staff - ARABIA - Districts and Towns.jpg",
        "target_width": 3200,
        "sha256": "45a2389ed00ddd50e67fead0db2a605374987a310ffc992f52136d1d72c3cac9",
    },
    {
        "filename": "thomas-1931-rub-al-khali.jpg",
        "commons_title": "File:Oman 1930.jpg",
        "target_width": 3200,
        "sha256": "3c19d3ea7ab0dfbd0b0ba30a92854bebb3b180383adfa6f3a776b9844b5f6783",
    },
    {
        "filename": "berghaus-1835-arabia.jpg",
        "commons_title": "File:خريطة لجزيرة العرب وبلاد النيل.jpg",
        "target_width": 3200,
        "sha256": "0a1e629807a352062d737deb188b8f5c2c22d63fa356cade2ea8282bd01a66b1",
    },
    {
        "filename": "arabia-600ce-tribes.png",
        "commons_title": "File:Map of Arabia 600 AD-ar.svg",
        "target_width": 2200,
        "sha256": "1ba4b92f1f97280a0849c468a5cfcf3030dead14ca3d280b513fe537513a1aa6",
    },
    {
        "filename": "yemen-south-arabia-tribes.png",
        "commons_title": "File:مناطق قبائل اليمن وجنوب الجزيرة العربية.png",
        "target_width": None,
        "sha256": "157f02f2616bead36bea12de9946b3777a957806902ffb203a164fa058051e66",
    },
    {
        "filename": "ottoman-asir-pre1919.jpg",
        "commons_title": "File:خريطة عثمانية لقبائل جنوب غرب الجزيرة العربية - آل سعد بللسمر.jpg",
        "target_width": None,
        "sha256": "e116f47cc1d73432f88d2d372150a1ad8217b44b3b63e63b297dd9d8ed17b9d6",
    },
    {
        "filename": "iraq-1930-hashimi.png",
        "commons_title": "File:خارطة توزيع القبائل في العراق عام 1930.png",
        "target_width": None,
        "sha256": "f6963be7a630a99d009206d5ad0122d3e02467fc01c573bd73cc794d09584268",
    },
]


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def fetch_bytes(url: str) -> bytes:
    req = urllib.request.Request(url, headers={"User-Agent": UA})
    with urllib.request.urlopen(req, timeout=60) as resp:
        return resp.read()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--dest",
        type=Path,
        default=Path(".data/source-storage/sources/plates"),
        help="Destination directory for plate images",
    )
    parser.add_argument("--verify-only", action="store_true", help="Verify existing files without downloading")
    args = parser.parse_args()
    args.dest.mkdir(parents=True, exist_ok=True)

    failed = False
    for entry in PLATES:
        target = args.dest / entry["filename"]
        if target.exists():
            digest = sha256_file(target)
            if digest == entry["sha256"]:
                print(f"OK   {entry['filename']} (verified)")
                continue
            if args.verify_only:
                print(f"FAIL {entry['filename']} checksum mismatch: {digest}", file=sys.stderr)
                failed = True
                continue
        elif args.verify_only:
            print(f"MISS {entry['filename']}", file=sys.stderr)
            failed = True
            continue

        params = {
            "action": "query",
            "titles": entry["commons_title"],
            "prop": "imageinfo",
            "iiprop": "url|size|mime",
            "format": "json",
        }
        if entry["target_width"]:
            params["iiurlwidth"] = str(entry["target_width"])
        meta = json.loads(fetch_bytes("https://commons.wikimedia.org/w/api.php?" + urllib.parse.urlencode(params)).decode("utf-8"))
        page = next(iter(meta["query"]["pages"].values()))
        ii = page["imageinfo"][0]
        dl_url = ii.get("thumburl") if (entry["target_width"] and ii.get("thumburl")) else ii["url"]
        data = fetch_bytes(dl_url)
        digest = hashlib.sha256(data).hexdigest()
        target.write_bytes(data)
        status = "OK  " if digest == entry["sha256"] else "WARN"
        print(f"{status} {entry['filename']} ({len(data):,} bytes, sha256={digest[:12]})")
        if digest != entry["sha256"]:
            print(f"     expected {entry['sha256']}, got {digest}", file=sys.stderr)
            failed = True

    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
