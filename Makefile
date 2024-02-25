##
# memcache
#
# @file
# @version 0.1

build:
	go build -o memcache .
run: build
	chmod +x ./memcache
	./memcache



# end
