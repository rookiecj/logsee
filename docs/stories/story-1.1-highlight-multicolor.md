# Story 1.1 — Highlight 멀티컬러

## 요구사항

- 하이라이트 적용 검색어에서 **토큰마다 배경색(ANSI 256)만** 지정할 수 있다. 전경은 고정(가독성용 기본색).
- 문법: `"needle"#<0–255>` 또는 `"needle"#<name>` (큰따옴표로 감싼 needle 직후 `#`, 공백 없음). 필터와 동일하게 ASCII 공백/탭으로 토큰 구분.
- **이름 색**: 내장 팔레트 + `config.toml`의 `highlight_color_names`로 사용자 정의. **동일 키는 config가 우선**.
- **겹침**: 한 바이트 위치에 여러 needle이 겹치면 **검색어에서 뒤에 나온 토큰이 우선**.
- **무효 색**(범위 밖 숫자·정의되지 않은 이름): 해당 토큰만 매칭/강조에서 제외.
- **하위 호환**: `#` 없는 기존 검색어는 이전과 동일(기본 배경 214 / 전경 0).

## Acceptance criteria

- [x] 위 문법으로 멀티컬러 하이라이트가 동작한다.
- [x] `config.toml`에 이름→256 값 매핑이 반영된다.
- [x] `make test` 통과 (유닛 테스트 포함).

## 관련 계획

- [highlight-multicolor-plan.md](../plans/highlight-multicolor-plan.md)
