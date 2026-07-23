;; munge / namespace-munge (fundamentals batch A2): the Compiler CHAR_MAP
;; over (str s) — symbols stay symbols, everything else becomes a string;
;; '.' survives, '-' becomes a bare '_'. namespace-munge replaces ONLY
;; hyphens and always returns a string. The inverse table lives in
;; clojure.repl/demunge (core/repl.cljg).
;; oracle (clojure 1.12.5, `clojure -M` 2026-07-23):
;;   (munge "foo-bar?") => "foo_bar_QMARK_"
;;   (munge 'foo-bar?) => foo_bar_QMARK_  (a symbol)
;;   (symbol? (munge 'a-b)) => true
;;   (munge :kw) => "_COLON_kw"
;;   (munge "") => ""
;;   (munge "a-b.c+d>e<f=g~h!i@j#k'l\"m%n^o&p*q|r{s}t[u]v/w\\x?y:z")
;;     => "a_b.c_PLUS_d_GT_e_LT_f_EQ_g_TILDE_h_BANG_i_CIRCA_j_SHARP_k_SINGLEQUOTE_l_DOUBLEQUOTE_m_PERCENT_n_CARET_o_AMPERSAND_p_STAR_q_BAR_r_LBRACE_s_RBRACE_t_LBRACK_u_RBRACK_v_SLASH_w_BSLASH_x_QMARK_y_COLON_z"
;;   (namespace-munge "my-ns.core") => "my_ns.core"
;;   (namespace-munge 'my-ns.core) => "my_ns.core"
;;   (namespace-munge "a-b_c.d") => "a_b_c.d"
[(munge "foo-bar?")
 (munge 'foo-bar?)
 (symbol? (munge 'a-b))
 (munge :kw)
 (munge "")
 (munge "a-b.c+d>e<f=g~h!i@j#k'l\"m%n^o&p*q|r{s}t[u]v/w\\x?y:z")
 (namespace-munge "my-ns.core")
 (namespace-munge 'my-ns.core)
 (namespace-munge "a-b_c.d")]
;; expect: ["foo_bar_QMARK_" foo_bar_QMARK_ true "_COLON_kw" "" "a_b.c_PLUS_d_GT_e_LT_f_EQ_g_TILDE_h_BANG_i_CIRCA_j_SHARP_k_SINGLEQUOTE_l_DOUBLEQUOTE_m_PERCENT_n_CARET_o_AMPERSAND_p_STAR_q_BAR_r_LBRACE_s_RBRACE_t_LBRACK_u_RBRACK_v_SLASH_w_BSLASH_x_QMARK_y_COLON_z" "my_ns.core" "my_ns.core" "a_b_c.d"]
