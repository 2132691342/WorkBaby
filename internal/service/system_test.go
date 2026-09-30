// 人设与提示词：输出风格规则必须一直在位。
//
// 真实问题：人设只写了工作原则，没有任何输出风格约束，模型自由发挥，
// 回复里塞满 emoji 和客套套话，界面看上去一眼就是「AI 生成的」。
// 这类规则不在代码里就在提示词里，删掉不会有编译错误，所以要用测试钉住。
package service

import (
	"strings"
	"testing"
)

func TestPersonaForbidsEmojiAndFiller(t *testing.T) {
	// 少一条就会退回 AI 味，提示词里的 emoji 禁令尤其容易被当成冗余删掉。
	required := []string{
		"不要用 emoji",
		"客套开场",
		"套话收尾",
		"不确定",
	}
	for _, want := range required {
		if !strings.Contains(persona, want) {
			t.Fatalf("人设里少了输出风格规则：%q", want)
		}
	}
}

func TestPersonaKeepsWorkPrinciples(t *testing.T) {
	required := []string{
		"先看清再动手",
		"说人话",
		"不编造",
	}
	for _, want := range required {
		if !strings.Contains(persona, want) {
			t.Fatalf("人设里少了工作原则：%q", want)
		}
	}
}
