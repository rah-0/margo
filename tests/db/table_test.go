package db_test

import (
	"strconv"
	"testing"

	"github.com/rah-0/margo/db"
)

type normalizeStringCase struct {
	input    string
	expected string
}

type normalizeStringGroup struct {
	name  string
	cases []normalizeStringCase
}

func TestNormalizeString(t *testing.T) {
	t.Parallel()

	groups := []normalizeStringGroup{
		{"empty and single characters", []normalizeStringCase{
			{"", ""},
			{"a", "A"},
			{"A", "A"},
			{"1", "1"},
			{"٢", "٢"},
			{"!", "!"},
			{"é", "É"},
			{"ı", "I"},
			{"ǅ", "Ǆ"},
			{"東", "東"},
			{"\xff", "\uFFFD"},
		}},
		{"separators", []normalizeStringCase{
			{"table_name", "TableName"},
			{"Table_Name", "TableName"},
			{"entity_user_data", "EntityUserData"},
			{"user_id", "UserId"},
			{"user1_data2", "User1Data2"},
			{"_table_name", "TableName"},
			{"table_name_", "TableName"},
			{"__table__name__", "TableName"},
			{"table__name", "TableName"},
			{"user___id", "UserId"},
			{"user-name", "UserName"},
			{"user.name", "UserName"},
			{"user--name", "UserName"},
			{"user..name", "UserName"},
			{"-user-name-", "UserName"},
			{".user.name.", "UserName"},
			{"user_-..name", "UserName"},
			{"_-.user._-name.-_", "UserName"},
			{"ip_address_v4", "IpAddressV4"},
			{"cpu_temp_stats", "CpuTempStats"},
			{"user2fa_status", "User2faStatus"},
			{"v2_api_users", "V2ApiUsers"},
			{"___", ""},
			{"__", ""},
			{"_", ""},
			{"-", ""},
			{".", ""},
			{"_-. ", ""},
		}},
		{"separators bypass all camel and digit splitting", []normalizeStringCase{
			{"userID_stats", "UseridStats"},
			{"_userID", "Userid"},
			{"userID-Stats", "UseridStats"},
			{"userID.Stats", "UseridStats"},
			{"userID stats", "UseridStats"},
			{"userID ", "Userid"},
			{" userID", "Userid"},
			{"userID  stats", "UseridStats"},
			{"  ", ""},
			{"user2fa beta3go", "User2faBeta3go"},
			{"foo!bar baz", "Foo!barBaz"},
			{"東京name next", "東京nameNext"},
			{"foo\tbar_baz", "Foo\tbarBaz"},
		}},
		{"camel case and acronyms", []normalizeStringCase{
			{"tableName", "TableName"},
			{"TableName", "TableName"},
			{"TABLENAME", "Tablename"},
			{"userID", "UserId"},
			{"userIDStats", "UserIdStats"},
			{"HTTPConnection", "HttpConnection"},
			{"OAuthToken", "OAuthToken"},
			{"ab", "Ab"},
			{"AB", "Ab"},
			{"ABc", "ABc"},
			{"ABCd", "AbCd"},
			{"ABCDx", "AbcDx"},
			{"aBCd", "ABCd"},
			{"aBcD", "ABcD"},
			{"xY", "XY"},
			{"XMLHttpRequest", "XmlHttpRequest"},
			{"JSONDataAPI", "JsonDataApi"},
			{"Aİ", "Ai"},
		}},
		{"numbers", []normalizeStringCase{
			{"user2fa", "User2Fa"},
			{"123abc", "123Abc"},
			{"abc123def", "Abc123Def"},
			{"foo٢bar", "Foo٢Bar"},
			{"v42HTTPServer", "V42HttpServer"},
			{"ID2HTTP", "Id2Http"},
			{"foo१२bar", "Foo१२Bar"},
			{"foo９bar", "Foo９Bar"},
			{"foo2٢bar", "Foo2٢Bar"},
			{"foo²bar", "Foo²Bar"},
			{"fooⅫbar", "FooⅫBar"},
		}},
		{"unicode letters and combining marks", []normalizeStringCase{
			{"tést_tab", "TéstTab"},
			{"über_cool", "ÜberCool"},
			{"данные_пользователя", "ДанныеПользователя"},
			{"BöseÜberraschung", "BöseÜberraschung"},
			{"ǅelta", "ǄElta"},
			{"ǅǅ", "Ǆǆ"},
			{"東京name", "東京Name"},
			{"a!Ⅻb", "A!ⅻB"},
			{"e\u0301clair", "E\u0301Clair"},
			{"éclair", "Éclair"},
			{"aǅb", "AǄB"},
			{"東京語", "東京語"},
			{"a\u0301b", "A\u0301B"},
			{"a\u0301\u0308b", "A\u0301\u0308B"},
		}},
		{"retained punctuation and whitespace", []normalizeStringCase{
			{"tableName!", "TableName!"},
			{"foo!bar", "Foo!Bar"},
			{"foo!!BAR", "Foo!!Bar"},
			{"foo\tbar", "Foo\tBar"},
			{"foo\nbar", "Foo\nBar"},
			{"foo\u00a0bar", "Foo\u00a0Bar"},
			{"foo\x00bar", "Foo\x00Bar"},
			{"!foo", "!Foo"},
			{"foo/bar", "Foo/Bar"},
			{"foo💡bar", "Foo💡Bar"},
			{"\tHTTPServer", "\tHttpServer"},
		}},
		{"UTF-8 decoding", []normalizeStringCase{
			{"userID\xffABC", "Userid\uFFFDabc"},
			{"\xffuserID", "\uFFFDuserid"},
			{"userID\xff", "Userid\uFFFD"},
			{"userID\xc3", "Userid\uFFFD"},
			{"userID\xff_statsABC", "Userid\uFFFDStatsabc"},
			{"userID_\xffABC", "Userid\uFFFDabc"},
			{"\xffABC userID", "\uFFFDabcUserid"},
			{"foo\xc0\xafBAR", "Foo\uFFFD\uFFFDbar"},
			{"\xc3_\xa9", "\uFFFD\uFFFD"},
			{"userID\uFFFDABC", "UserId\uFFFDAbc"},
		}},
	}

	for _, group := range groups {
		t.Run(group.name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range group.cases {
				t.Run(strconv.Quote(tc.input), func(t *testing.T) {
					t.Parallel()
					if result := db.NormalizeString(tc.input); result != tc.expected {
						t.Errorf("NormalizeString(%q) = %q; want %q", tc.input, result, tc.expected)
					}
				})
			}
		})
	}
}
