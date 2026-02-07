set title 'Chunk size distribution (cumulative)'
set key right bottom
# set logscale x 2
set xlabel 'Chunk size (bytes)'
set term png
set output 'cumulative-sizes.png'
set datafile separator ','
plot 'cumulative-sizes.csv' using 1:2 title 'Before deduplication' with lines, \
     '' using 1:3 title 'After deduplication' with lines
