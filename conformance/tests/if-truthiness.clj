;; Only nil and false are falsey; 0, "" and empty collections are truthy.
[(if nil :t :f)
 (if false :t :f)
 (if 0 :t :f)
 (if "" :t :f)
 (if [] :t :f)]
;; expect: [:f :f :t :t :t]
