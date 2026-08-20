package game

import "testing"

func TestTranslateUnoAction(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want string
	}{
		{
			name: "reverse",
			msg:  "Turn order has been reversed! \n",
			want: "出牌顺序已反转！\n",
		},
		{
			name: "skip",
			msg:  "Alice's turn skipped! \n",
			want: "Alice 的回合被跳过！\n",
		},
		{
			name: "reverse and skip",
			msg:  "Turn order has been reversed! \nAlice's turn skipped! \n",
			want: "出牌顺序已反转！\nAlice 的回合被跳过！\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := translateUnoAction(tt.msg); got != tt.want {
				t.Fatalf("translateUnoAction() = %q, want %q", got, tt.want)
			}
		})
	}
}
