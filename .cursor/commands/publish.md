# /publish

배포 전 자동 검사 후 **시맨틱 버전 증가**, **변경분 전체 커밋**, **`v*` 태그 생성**, **원격 push**까지 한 번에 실행합니다.

## 실행

저장소 루트에서 — **기본은 마이너** (`x.y.z` → `x.(y+1).0`):

```bash
make publish
```

**패치만** 올릴 때 (`0.1.2` → `0.1.3`):

```bash
make publish BUMP=patch
```

**메이저** (`0.1.2` → `1.0.0`):

```bash
make publish BUMP=major
```

스크립트 직접 호출: `./scripts/publish.sh` (기본 minor), `./scripts/publish.sh patch` 등

## 버전 규칙

| BUMP   | 예 (`0.1.2` 기준) |
|--------|-------------------|
| `minor` (기본) | `0.2.0` |
| `patch`        | `0.1.3` |
| `major`        | `1.0.0` |

## 동작 순서

1. **`make publish-verify`**: `fmt-check` → `lint`(go vet) → `test` → `build` — 하나라도 실패하면 중단
2. **`VERSION` 파일** 갱신
3. **`git add -A`** 후 **`chore: release v…`** 커밋
4. **annotated 태그** `vX.Y.Z` 생성
5. **현재 브랜치**와 **태그**를 `origin`에 push

## 참고

- `gofmt`가 필요하면 먼저 `make fmt`로 맞춘 뒤 다시 실행하세요.
- 추적하지 않을 파일은 `.gitignore`에 두면 `git add -A`에 포함되지 않습니다.
- GitHub Actions의 **Release** 워크플로는 `v*` 태그 push 시 바이너리를 빌드합니다 (`.github/workflows/release.yml`).
