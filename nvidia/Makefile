run_8192:
	@printf "thread\ttruth\tsimulation\n"
	@for t in 480 512 544 576 608 640 672 704 736; do \
		gt_file="data/simtune-example-8192-$$t/gt"; \
		truth=$$(cat $$gt_file); \
		sim=$$(./nvidia -trace-dir data/simtune-example-8192-$$t); \
		printf "%s\t%s\t%s\n" "$$t" "$$truth" "$$sim"; \
	done
	@rm akita_sim_d*

run_8192_small:
	@printf "thread\ttruth\tsimulation\n"
	@for t in 480 512 544 576; do \
		gt_file="data/simtune-example-8192-$$t/gt"; \
		truth=$$(cat $$gt_file); \
		sim=$$(./nvidia -trace-dir data/simtune-example-8192-$$t); \
		printf "%s\t%s\t%s\n" "$$t" "$$truth" "$$sim"; \
	done
	@rm akita_sim_d*

run_8192_large:
	@printf "thread\ttruth\tsimulation\n"
	@for t in 640 672 704 736; do \
		gt_file="data/simtune-example-8192-$$t/gt"; \
		truth=$$(cat $$gt_file); \
		sim=$$(./nvidia -trace-dir data/simtune-example-8192-$$t); \
		printf "%s\t%s\t%s\n" "$$t" "$$truth" "$$sim"; \
	done
	@rm akita_sim_d*