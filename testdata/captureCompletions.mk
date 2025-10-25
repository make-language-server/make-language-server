snake_glue_function = $1_$2
glued_variable := $(call snake_glue_function,head,tail)
define target_echo_template
$1:
	echo $$@ $(glued_variable)
endef
$(eval $(call target_echo_template,echo_target))
