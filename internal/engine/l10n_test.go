package engine

import "testing"

func TestPreferredLanguagesFromEnvironment(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want Lang
	}{
		{map[string]string{"LANG": "ko_KR.UTF-8"}, KO},
		{map[string]string{"LANG": "zh_CN.UTF-8"}, ZH},
		{map[string]string{"LANG": "en_US.UTF-8", "LANGUAGE": "zh-Hans:en"}, ZH},
		{map[string]string{"LANG": "zh_CN.UTF-8", "LC_ALL": "C", "LANGUAGE": "zh"}, EN},
		{map[string]string{"LANG": "en_US.UTF-8"}, EN},
		{map[string]string{}, EN},
		{map[string]string{"LANG": "en_US.UTF-8", "LC_MESSAGES": "ko_KR.UTF-8"}, KO},
		{map[string]string{"LC_MESSAGES": "ko_KR.UTF-8", "LC_ALL": "en_US.UTF-8"}, EN},
		{map[string]string{"LANG": "en_US.UTF-8", "LANGUAGE": "ko:en"}, KO},
		{map[string]string{"LANG": "ko_KR.UTF-8", "LANGUAGE": "en_GB:ko"}, EN},
		{map[string]string{"LANG": "C.UTF-8", "LANGUAGE": "ko"}, EN}, // LANGUAGE is ignored for the C locale
		{map[string]string{"LC_ALL": "C", "LANGUAGE": "ko"}, EN},
		{map[string]string{"LANGUAGE": "ko"}, EN}, // no locale at all means C
		{map[string]string{"LANG": "ja_JP.UTF-8"}, EN},
	}
	for _, c := range cases {
		got := DetectLanguage(PreferredLanguages(func(k string) string { return c.env[k] }))
		if got != c.want {
			t.Errorf("%v → %s, want %s", c.env, got, c.want)
		}
	}
}

func TestParseLang(t *testing.T) {
	for in, want := range map[string]bool{"ko": true, "en": true, "zh": true, "kr": false, "": false, "KO": false} {
		if _, ok := ParseLang(in); ok != want {
			t.Errorf("ParseLang(%q) ok = %v", in, ok)
		}
	}
}
